package hid

/*
#cgo linux CFLAGS: -DDEFAULT_VISIBILITY="" -DOS_LINUX -D_GNU_SOURCE -DPOLL_NFDS_TYPE=int
#cgo linux,!android LDFLAGS: -lrt

#include <poll.h>
#include "hidapi_linux.h"
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Devices returns a channel that will receive a DeviceInfo struct for each HID device.
func Devices() <-chan *DeviceInfo {
	result := make(chan *DeviceInfo)
	go func() {
		enumerateLock.Lock()
		defer enumerateLock.Unlock()
		defer close(result)

		// Gather all device infos and ensure they are freed before returning
		head := C.hid_enumerate(C.ushort(0), C.ushort(0))
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

	device *C.hid_device // Low level HID device to communicate through
	lock   sync.Mutex
}

// Close releases the HID USB device handle.
func (dev *linuxDevice) Close() {
	dev.lock.Lock()
	defer dev.lock.Unlock()

	if dev.device != nil {
		C.hid_close(dev.device)
		dev.device = nil
	}
	return
}

// Write sends an output report to a HID device.
//
// Write will send the data on the first OUT endpoint, if one exists. If it does
// not, it will send the data through the Control Endpoint (Endpoint 0).
func (dev *linuxDevice) Write(b []byte) error {
	// Abort if nothing to write
	if len(b) == 0 {
		return nil
	}
	// Abort if device closed in between
	dev.lock.Lock()
	device := dev.device
	dev.lock.Unlock()

	if device == nil {
		return errDeviceClosed
	}
	report := b

	// Execute the write operation
	written := int(C.hid_write(device, (*C.uchar)(&report[0]), C.size_t(len(report))))
	if written == -1 {
		// If write failed, verify if closed or other error
		dev.lock.Lock()
		device = dev.device
		dev.lock.Unlock()

		if device == nil {
			return errDeviceClosed
		}
		// Device not closed, some other error occurred
		message := C.hid_error(device)
		if message == nil {
			return errors.New("hidapi: unknown failure")
		}
		failure, _ := wcharTToString(message)
		return errors.New("hidapi: " + failure)
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
	// Abort if nothing to write
	if len(b) == 0 {
		return nil
	}
	// Abort if device closed in between
	dev.lock.Lock()
	device := dev.device
	dev.lock.Unlock()

	if device == nil {
		return errDeviceClosed
	}

	// Send the feature report
	written := int(C.hid_send_feature_report(device, (*C.uchar)(&b[0]), C.size_t(len(b))))
	if written == -1 {
		// If write failed, verify if closed or other error
		dev.lock.Lock()
		device = dev.device
		dev.lock.Unlock()

		if device == nil {
			return errDeviceClosed
		}
		// Device not closed, some other error occurred
		message := C.hid_error(device)
		if message == nil {
			return errors.New("hidapi: unknown failure")
		}
		failure, _ := wcharTToString(message)
		return errors.New("hidapi: " + failure)
	}
	return nil
}

func (dev *linuxDevice) WriteInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

// Read retrieves an input report from a HID device.
func (dev *linuxDevice) Read(b []byte) (int, error) {
	// Abort if nothing to read
	if len(b) == 0 {
		return 0, nil
	}
	// Abort if device closed in between
	dev.lock.Lock()
	device := dev.device
	dev.lock.Unlock()

	if device == nil {
		return 0, errDeviceClosed
	}
	// Execute the read operation
	read := int(C.hid_read(device, (*C.uchar)(&b[0]), C.size_t(len(b))))
	if read == -1 {
		// If the read failed, verify if closed or other error
		dev.lock.Lock()
		device = dev.device
		dev.lock.Unlock()

		if device == nil {
			return 0, errDeviceClosed
		}
		// Device not closed, some other error occurred
		message := C.hid_error(device)
		if message == nil {
			return 0, errors.New("hidapi: unknown failure")
		}
		failure, _ := wcharTToString(message)
		return 0, errors.New("hidapi: " + failure)
	}
	return read, nil
}

// ReadFeature retrieves a feature report from a HID device
//
// Set the first byte of []b to the Report ID of the report to be read. Make
// sure to allow space for this extra byte in []b. Upon return, the first byte
// will still contain the Report ID, and the report data will start in b[1].
func (dev *linuxDevice) ReadFeature(b []byte) (int, error) {
	// Abort if we don't have anywhere to write the results
	if len(b) == 0 {
		return 0, nil
	}
	// Abort if device closed in between
	dev.lock.Lock()
	device := dev.device
	dev.lock.Unlock()

	if device == nil {
		return 0, errDeviceClosed
	}

	// Retrieve the feature report
	read := int(C.hid_get_feature_report(device, (*C.uchar)(&b[0]), C.size_t(len(b))))
	if read == -1 {
		// If the read failed, verify if closed or other error
		dev.lock.Lock()
		device = dev.device
		dev.lock.Unlock()

		if device == nil {
			return 0, errDeviceClosed
		}

		// Device not closed, some other error occurred
		message := C.hid_error(device)
		if message == nil {
			return 0, errors.New("hidapi: unknown failure")
		}
		failure, _ := wcharTToString(message)
		return 0, errors.New("hidapi: " + failure)
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
