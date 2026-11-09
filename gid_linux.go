package gid

/*
#cgo linux CFLAGS: -DDEFAULT_VISIBILITY="" -DOS_LINUX -D_GNU_SOURCE -DPOLL_NFDS_TYPE=int
#cgo linux,!android LDFLAGS: -lrt

#include <poll.h>
#include "hidapi_linux.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Devices returns a channel that will receive a DeviceInfo struct for each HID device.
func Devices() <-chan *DeviceInfo {
	return devicesByID(0, 0)
}

// devicesByID narrows HIDAPI's string-query opens before the Go equality filter.
// HIDAPI treats zero as a wildcard candidate; FindDevices still returns exact
// IDs, including zero. Devices intentionally retains the full enumeration.
func devicesByID(vendor, product uint16) <-chan *DeviceInfo {
	result := make(chan *DeviceInfo, maxDeviceChannelSize)
	go func() {
		enumerateLock.Lock()
		defer enumerateLock.Unlock()
		defer close(result)

		// Gather all device infos and ensure they are freed before returning
		head := C.hid_enumerate(C.ushort(vendor), C.ushort(product))
		if head == nil {
			return
		}
		defer C.hid_free_enumeration(head)

		// Iterate the list and retrieve the device details
		for ; head != nil; head = head.next {
			info := DeviceInfo{
				Path:          C.GoString(head.path),
				VendorID:      uint16(head.vendor_id),
				ProductID:     uint16(head.product_id),
				VersionNumber: uint16(head.release_number),
			}
			if head.serial_number != nil {
				info.SerialNumber, _ = wcharTToString(head.serial_number)
			}
			if head.product_string != nil {
				info.Product, _ = wcharTToString(head.product_string)
			}
			if head.manufacturer_string != nil {
				info.Manufacturer, _ = wcharTToString(head.manufacturer_string)
			}
			result <- &info
		}
	}()
	return result
}

// ByPath returns a device by its path.
func ByPath(path string) (*DeviceInfo, error) {
	ch := Devices()
	defer drainDevices(ch)

	for d := range ch {
		if d.Path == path {
			return d, nil
		}
	}
	return nil, errDeviceNotFound
}

// Open connects to an HID device by its path name.
func (di *DeviceInfo) Open() (Device, error) {
	enumerateLock.Lock()
	defer enumerateLock.Unlock()

	path := C.CString(di.Path)
	defer C.free(unsafe.Pointer(path))

	device := C.hid_open_path(path)
	if device == nil {
		return nil, errors.New("hidapi: failed to open device")
	}
	return &linuxDevice{
		DeviceInfo: di,
		device:     device,
	}, nil
}

// Device is a live HID USB connected device handle.
type linuxDevice struct {
	*DeviceInfo // Embed the infos for easier access

	device   *C.hid_device // Protected by a lifetime lease, or exclusive Close.
	lifetime linuxLifetime
}

// Close rejects new I/O, waits for active native calls, and releases the HID
// handle once. It has no hard deadline: native calls and hid_close's read-thread
// join can still block. Concurrent Close callers wait for the same disposal.
func (dev *linuxDevice) Close() {
	dev.lifetime.close(func() {
		if dev.device != nil {
			C.hid_close(dev.device)
			dev.device = nil
		}
	})
}

// acquireDevice returns a pinned handle. The caller must retain the lease until
// the native operation and any native error-string conversion have finished.
func (dev *linuxDevice) acquireDevice() (*C.hid_device, error) {
	if !dev.lifetime.acquire() {
		return nil, errDeviceClosed
	}
	if dev.device == nil {
		dev.lifetime.release()
		return nil, errDeviceClosed
	}
	return dev.device, nil
}

// linuxNativeError may only be called while the native handle is pinned.
func linuxNativeError(device *C.hid_device) error {
	message := C.hid_error(device)
	if message == nil {
		return errors.New("hidapi: unknown failure")
	}
	failure, _ := wcharTToString(message)
	return errors.New("hidapi: " + failure)
}

// Write sends an output report to a HID device.
//
// Write will send the data on the first OUT endpoint, if one exists. If it does
// not, it will send the data through the Control Endpoint (Endpoint 0).
func (dev *linuxDevice) Write(b []byte) error {
	device, err := dev.acquireDevice()
	if err != nil {
		return err
	}
	defer dev.lifetime.release()
	if err := validateReportBuffer(b); err != nil {
		return err
	}
	// Execute the write operation
	written := int(C.hid_write(device, (*C.uchar)(&b[0]), C.size_t(len(b))))
	if written == -1 {
		return linuxNativeError(device)
	}

	return nil
}

// WriteFeature sends a feature report to a HID device
//
// Feature reports are sent over the Control endpoint as a Set_Report transfer.
// The first byte of b must contain the Report ID. For devices which only
// support a single report, this must be set to 0x0. The remaining bytes
// contain the report data. Since the Report ID is mandatory, calls to
// SendFeatureReport() will always contain one more byte than the report
// contains. For example, if a hid report is 16 bytes long, 17 bytes must be
// passed to SendFeatureReport(): the Report ID (or 0x0, for devices
// which do not use numbered reports), followed by the report data (16 bytes).
// In this example, the length passed in would be 17.
func (dev *linuxDevice) WriteFeature(b []byte) error {
	device, err := dev.acquireDevice()
	if err != nil {
		return err
	}
	defer dev.lifetime.release()
	if err := validateReportBuffer(b); err != nil {
		return err
	}

	// Send the feature report
	written := int(C.hid_send_feature_report(device, (*C.uchar)(&b[0]), C.size_t(len(b))))
	if written == -1 {
		return linuxNativeError(device)
	}
	return nil
}

func (dev *linuxDevice) WriteInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

// Read blocks until an input report, a native error, or Close. Finite native
// wait slices are internal; an idle slice is not reported as a successful read.
func (dev *linuxDevice) Read(b []byte) (int, error) {
	device, err := dev.acquireDevice()
	if err != nil {
		return 0, err
	}
	defer dev.lifetime.release()
	// HIDAPI input reads receive reports rather than selecting a report ID.
	if len(b) == 0 {
		return 0, errEmptyReportBuffer
	}
	// A finite native wait lets this read return its lease after Close signals.
	// Other operations retain concurrent read leases, so a write that generates
	// input can still make progress while this call waits for that input.
	return linuxPollInput(&dev.lifetime, func(milliseconds int) (int, error) {
		n := int(C.hid_read_timeout(device, (*C.uchar)(&b[0]), C.size_t(len(b)), C.int(milliseconds)))
		if n == -1 {
			return 0, linuxNativeError(device)
		}
		return n, nil
	})
}

// ReadFeature retrieves a feature report from a HID device
//
// Set the first byte of []b to the Report ID of the report to be read. Make
// sure to allow space for this extra byte in []b. Upon return, the first byte
// will still contain the Report ID, and the report data will start in b[1].
func (dev *linuxDevice) ReadFeature(b []byte) (int, error) {
	device, err := dev.acquireDevice()
	if err != nil {
		return 0, err
	}
	defer dev.lifetime.release()
	if err := validateReportBuffer(b); err != nil {
		return 0, err
	}

	// Retrieve the feature report
	read := int(C.hid_get_feature_report(device, (*C.uchar)(&b[0]), C.size_t(len(b))))
	if read == -1 {
		return 0, linuxNativeError(device)
	}

	return read, nil
}

func (dev *linuxDevice) ReadInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

// Supported returns whether this platform is supported by the HID library or not.
// The goal of this method is to allow programmatically handling platforms that do
// not support USB HID and not having to fall back to build constraints.
func Supported() bool {
	return true
}
