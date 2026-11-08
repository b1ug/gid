package gid

/*
#cgo LDFLAGS: -lSetupapi -lhid

#ifdef __MINGW32__
#include <ntdef.h>
#endif

#include <windows.h>
#include <setupapi.h>
#include <hidsdi.h>
#include <hidclass.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// Windows handles are opaque integer values, not Go pointers. Keep the
// integer-to-HANDLE conversion in C so Go's GC never sees a fake pointer.
static DWORD gid_last_error(void) {
    DWORD error = GetLastError();
    return error ? error : ERROR_GEN_FAILURE;
}

static DWORD gid_get_overlapped_result(uintptr_t handle, LPOVERLAPPED overlapped, LPDWORD transferred, BOOL wait) {
    return GetOverlappedResult((HANDLE)handle, overlapped, transferred, wait) ? ERROR_SUCCESS : gid_last_error();
}

static DWORD gid_set_feature(uintptr_t handle, PVOID buffer, ULONG length) {
    return HidD_SetFeature((HANDLE)handle, buffer, length) ? ERROR_SUCCESS : gid_last_error();
}

// Each simultaneous I/O owns an event and an OVERLAPPED outside the Go heap.
// They remain allocated, together with the C-owned report buffer, until the
// operation's completion has been collected even when CancelIoEx was requested.
static LPOVERLAPPED gid_new_overlapped(DWORD *error) {
    LPOVERLAPPED overlapped = (LPOVERLAPPED)calloc(1, sizeof(OVERLAPPED));
    if (!overlapped) {
        *error = ERROR_NOT_ENOUGH_MEMORY;
        return NULL;
    }
    overlapped->hEvent = CreateEventW(NULL, TRUE, FALSE, NULL);
    if (!overlapped->hEvent) {
        *error = gid_last_error();
        free(overlapped);
        return NULL;
    }
    *error = ERROR_SUCCESS;
    return overlapped;
}

static void gid_free_overlapped(LPOVERLAPPED overlapped) {
    CloseHandle(overlapped->hEvent);
    free(overlapped);
}

static DWORD gid_write_overlapped(uintptr_t handle, PVOID buffer, DWORD length, LPOVERLAPPED overlapped) {
    DWORD transferred = 0;
    return WriteFile((HANDLE)handle, buffer, length, &transferred, overlapped) ? ERROR_SUCCESS : gid_last_error();
}

static DWORD gid_ioctl_overlapped(uintptr_t handle, DWORD report_type, PVOID buffer, DWORD length, LPOVERLAPPED overlapped) {
    DWORD transferred = 0;
    return DeviceIoControl((HANDLE)handle, report_type, buffer, length, buffer, length, &transferred, overlapped) ? ERROR_SUCCESS : gid_last_error();
}

static BOOLEAN gid_get_attributes(uintptr_t handle, PHIDD_ATTRIBUTES attributes) {
    return HidD_GetAttributes((HANDLE)handle, attributes);
}

static BOOLEAN gid_get_manufacturer_string(uintptr_t handle, PVOID buffer, ULONG length) {
    return HidD_GetManufacturerString((HANDLE)handle, buffer, length);
}

static BOOLEAN gid_get_product_string(uintptr_t handle, PVOID buffer, ULONG length) {
    return HidD_GetProductString((HANDLE)handle, buffer, length);
}

static BOOLEAN gid_get_serial_number_string(uintptr_t handle, PVOID buffer, ULONG length) {
    return HidD_GetSerialNumberString((HANDLE)handle, buffer, length);
}

static BOOLEAN gid_get_preparsed_data(uintptr_t handle, PHIDP_PREPARSED_DATA *data) {
    return HidD_GetPreparsedData((HANDLE)handle, data);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	ioctlHidGetInputReport = C.IOCTL_HID_GET_INPUT_REPORT
	ioctlHidGetFeature     = C.IOCTL_HID_GET_FEATURE
	ioctlHidSetFeature     = C.IOCTL_HID_SET_FEATURE
)

type winDevice struct {
	handle   syscall.Handle
	info     *DeviceInfo
	lifetime windowsLifetime
}

func validWindowsHandle(handle syscall.Handle) bool {
	return handle != 0 && handle != syscall.InvalidHandle
}

func (dev *winDevice) isValid() bool {
	_, err := dev.acquireHandle()
	if err != nil {
		return false
	}
	dev.lifetime.release()
	return true
}

// acquireHandle pins the handle through the entire native operation, completion
// wait, response copy and error conversion. Callers must release the lease.
func (dev *winDevice) acquireHandle() (syscall.Handle, error) {
	if !dev.lifetime.acquire() {
		return syscall.InvalidHandle, errDeviceClosed
	}
	if !validWindowsHandle(dev.handle) {
		dev.lifetime.release()
		return syscall.InvalidHandle, errDeviceClosed
	}
	return dev.handle, nil
}

// Close requests cancellation, then waits for all operations before closing the
// handle once. Cancellation is not completion and has no hard deadline; a
// synchronous HidD_SetFeature or an unresponsive driver can still block Close.
func (dev *winDevice) Close() {
	dev.lifetime.close(func() {
		if validWindowsHandle(dev.handle) {
			// CancelIoEx is available in the Go syscall API since Go 1.1. Ignore
			// ERROR_NOT_FOUND: active operations may already have completed.
			_ = syscall.CancelIoEx(dev.handle, nil)
		}
	}, func() {
		if validWindowsHandle(dev.handle) {
			_ = syscall.CloseHandle(dev.handle)
			dev.handle = syscall.InvalidHandle
		}
	})
}

func (dev *winDevice) Write(data []byte) error {
	handle, err := dev.acquireHandle()
	if err != nil {
		return err
	}
	defer dev.lifetime.release()
	// Windows requires the collection's complete output report size.
	outSize := int(dev.info.OutputReportLength)
	buffer, err := prepareWriteReport(data, outSize)
	if err != nil {
		return err
	}

	written, err := dev.overlapped(handle, buffer, 0, true)
	if err != nil {
		return err
	}
	if written != outSize {
		return errors.New("hid: written bytes mismatch")
	}
	return nil
}

func (dev *winDevice) WriteFeature(data []byte) error {
	handle, err := dev.acquireHandle()
	if err != nil {
		return err
	}
	defer dev.lifetime.release()
	buffer, err := prepareWriteReport(data, int(dev.info.FeatureReportLength))
	if err != nil {
		return err
	}

	// Reference: https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidsdi/nf-hidsdi-hidd_setfeature
	cBuffer := (C.PVOID)(unsafe.Pointer(&buffer[0]))
	if code := C.gid_set_feature(C.uintptr_t(handle), cBuffer, C.ULONG(len(buffer))); code != C.ERROR_SUCCESS {
		return fmt.Errorf("hid: set feature fail: %w", syscall.Errno(code))
	}
	return nil
}

func (dev *winDevice) WriteInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

func (dev *winDevice) Read(data []byte) (int, error) {
	handle, err := dev.acquireHandle()
	if err != nil {
		return 0, err
	}
	defer dev.lifetime.release()
	// Reference: https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidclass/ni-hidclass-ioctl_hid_get_input_report
	return dev.readReport(handle, data, ioctlHidGetInputReport, int(dev.info.InputReportLength))
}

func (dev *winDevice) ReadFeature(data []byte) (int, error) {
	handle, err := dev.acquireHandle()
	if err != nil {
		return 0, err
	}
	defer dev.lifetime.release()
	// Reference: https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidclass/ni-hidclass-ioctl_hid_get_feature
	return dev.readReport(handle, data, ioctlHidGetFeature, int(dev.info.FeatureReportLength))
}

func (dev *winDevice) ReadInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

// readReport is called with the lifetime lease retained by Read/ReadFeature.
func (dev *winDevice) readReport(handle syscall.Handle, data []byte, reportType uint32, minBufSize int) (int, error) {
	if err := validateReportBuffer(data); err != nil {
		return 0, err
	}
	if minBufSize <= 0 {
		return 0, errReportUnavailable
	}
	// A collection can contain reports smaller than its maximum report size.
	buf := data
	if len(data) < minBufSize {
		buf = make([]byte, minBufSize)
		copy(buf, data)
	}

	read, err := dev.overlapped(handle, buf, reportType, false)
	if err != nil {
		return 0, err
	}

	if reportType == ioctlHidGetInputReport {
		return copyInputReport(data, buf, read)
	}
	return copyReadReport(data, buf, read)
}

// overlapped keeps all asynchronously retained memory in C allocations. The
// caller retains a handle lease until this returns and response/error processing
// finishes. Requesting cancellation never releases those resources early.
func (dev *winDevice) overlapped(handle syscall.Handle, buffer []byte, reportType uint32, write bool) (int, error) {
	payload := C.CBytes(buffer)
	defer C.free(payload)
	var code C.DWORD
	ol := C.gid_new_overlapped(&code)
	if ol == nil {
		return 0, syscall.Errno(code)
	}
	defer C.gid_free_overlapped(ol)
	if write {
		code = C.gid_write_overlapped(C.uintptr_t(handle), payload, C.DWORD(len(buffer)), ol)
	} else {
		code = C.gid_ioctl_overlapped(C.uintptr_t(handle), C.DWORD(reportType), payload, C.DWORD(len(buffer)), ol)
	}
	if code != C.ERROR_SUCCESS && syscall.Errno(code) != syscall.ERROR_IO_PENDING {
		if write {
			return 0, syscall.Errno(code)
		}
		return 0, fmt.Errorf("hid: send device control fail: %w", syscall.Errno(code))
	}
	dev.lifetime.afterIssue(func() {
		_ = syscall.CancelIoEx(handle, (*syscall.Overlapped)(unsafe.Pointer(ol)))
	})
	var transferred C.DWORD
	if code = C.gid_get_overlapped_result(C.uintptr_t(handle), ol, &transferred, C.TRUE); code != C.ERROR_SUCCESS {
		if write {
			return 0, syscall.Errno(code)
		}
		return 0, fmt.Errorf("hid: retrieve result fail: %w", syscall.Errno(code))
	}
	if !write {
		if uint64(transferred) > uint64(len(buffer)) {
			return 0, errors.New("hid: invalid native report length")
		}
		C.memcpy(unsafe.Pointer(&buffer[0]), payload, C.size_t(len(buffer)))
	}
	return int(transferred), nil
}

type callCFn func(buf unsafe.Pointer, bufSize *C.DWORD) unsafe.Pointer

// simple helper function for this Windows "call a function twice to get the amount of space that needs to be allocated" stuff
func getCString(fnCall callCFn) string {
	var requiredSize C.DWORD
	fnCall(nil, &requiredSize)
	if requiredSize <= 0 {
		return ""
	}

	buffer := C.malloc((C.size_t)(requiredSize))
	defer C.free(buffer)

	strPt := fnCall(buffer, &requiredSize)

	return C.GoString((*C.char)(strPt))
}

func openDevice(info *DeviceInfo, enumerate bool) (*winDevice, error) {
	access := uint32(syscall.GENERIC_WRITE | syscall.GENERIC_READ)
	shareMode := uint32(syscall.FILE_SHARE_READ)
	if enumerate {
		// if we just need a handle to get the device properties
		// we should not claim exclusive access on the device
		access = 0
		shareMode = uint32(syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE)
	}
	pPtr, err := syscall.UTF16PtrFromString(info.Path)
	if err != nil {
		return nil, err
	}

	hFile, err := syscall.CreateFile(pPtr, access, shareMode, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	} else {
		return &winDevice{handle: hFile, info: info}, nil
	}
}

func getDeviceDetails(deviceInfoSet C.HDEVINFO, deviceInterfaceData *C.SP_DEVICE_INTERFACE_DATA) *DeviceInfo {
	devicePath := getCString(func(buffer unsafe.Pointer, size *C.DWORD) unsafe.Pointer {
		interfaceDetailData := (*C.SP_DEVICE_INTERFACE_DETAIL_DATA_A)(buffer)
		if interfaceDetailData != nil {
			interfaceDetailData.cbSize = C.DWORD(unsafe.Sizeof(interfaceDetailData))
		}
		C.SetupDiGetDeviceInterfaceDetailA(deviceInfoSet, deviceInterfaceData, interfaceDetailData, *size, size, nil)
		if interfaceDetailData != nil {
			return (unsafe.Pointer)(&interfaceDetailData.DevicePath[0])
		} else {
			return nil
		}
	})
	if devicePath == "" {
		return nil
	}

	// Make sure this device is of Setup Class "HIDClass" and has a driver bound to it.
	var i C.DWORD
	var devinfoData C.SP_DEVINFO_DATA
	devinfoData.cbSize = C.DWORD(unsafe.Sizeof(devinfoData))
	isHID := false
	for i = 0; ; i++ {
		if res := C.SetupDiEnumDeviceInfo(deviceInfoSet, i, &devinfoData); res == 0 {
			break
		}

		classStr := getCString(func(buffer unsafe.Pointer, size *C.DWORD) unsafe.Pointer {
			C.SetupDiGetDeviceRegistryPropertyA(deviceInfoSet, &devinfoData, C.SPDRP_CLASS, nil, (*C.BYTE)(buffer), *size, size)
			return buffer
		})

		if classStr == "HIDClass" {
			driverName := getCString(func(buffer unsafe.Pointer, size *C.DWORD) unsafe.Pointer {
				C.SetupDiGetDeviceRegistryPropertyA(deviceInfoSet, &devinfoData, C.SPDRP_DRIVER, nil, (*C.BYTE)(buffer), *size, size)
				return buffer
			})
			isHID = driverName != ""
			break
		}
	}

	if !isHID {
		return nil
	}
	d, _ := ByPath(devicePath)
	return d
}

// ByPath gets the device which is bound to the given path.
func ByPath(devicePath string) (*DeviceInfo, error) {
	devInfo := &DeviceInfo{Path: devicePath}
	dev, err := openDevice(devInfo, true)
	if err != nil {
		return nil, fmt.Errorf("hid: open device fail: %w", err)
	}
	defer dev.Close()
	if !dev.isValid() {
		return nil, errors.New("hid: failed to open device")
	}

	var attrs C.HIDD_ATTRIBUTES
	attrs.Size = C.DWORD(unsafe.Sizeof(attrs))
	C.gid_get_attributes(C.uintptr_t(dev.handle), &attrs)

	devInfo.VendorID = uint16(attrs.VendorID)
	devInfo.ProductID = uint16(attrs.ProductID)
	devInfo.VersionNumber = uint16(attrs.VersionNumber)

	const bufLen = 256
	buff := make([]uint16, bufLen)

	C.gid_get_manufacturer_string(C.uintptr_t(dev.handle), (C.PVOID)(&buff[0]), bufLen)
	devInfo.Manufacturer = syscall.UTF16ToString(buff)

	C.gid_get_product_string(C.uintptr_t(dev.handle), (C.PVOID)(&buff[0]), bufLen)
	devInfo.Product = syscall.UTF16ToString(buff)

	C.gid_get_serial_number_string(C.uintptr_t(dev.handle), (C.PVOID)(&buff[0]), bufLen)
	devInfo.SerialNumber = syscall.UTF16ToString(buff)

	var preparsedData C.PHIDP_PREPARSED_DATA
	if C.gid_get_preparsed_data(C.uintptr_t(dev.handle), &preparsedData) != 0 {
		var caps C.HIDP_CAPS

		if C.HidP_GetCaps(preparsedData, &caps) == C.HIDP_STATUS_SUCCESS {
			devInfo.InputReportLength = uint16(caps.InputReportByteLength)
			devInfo.FeatureReportLength = uint16(caps.FeatureReportByteLength)
			devInfo.OutputReportLength = uint16(caps.OutputReportByteLength)
		}

		C.HidD_FreePreparsedData(preparsedData)
	}

	return devInfo, nil
}

// Devices returns all HID devices which are connected to the system.
func Devices() <-chan *DeviceInfo {
	result := make(chan *DeviceInfo, maxDeviceChannelSize)
	go func() {
		enumerateLock.Lock()
		defer enumerateLock.Unlock()
		defer close(result)

		var InterfaceClassGuid C.GUID
		C.HidD_GetHidGuid(&InterfaceClassGuid)
		deviceInfoSet := C.SetupDiGetClassDevsA(&InterfaceClassGuid, nil, nil, C.DIGCF_PRESENT|C.DIGCF_DEVICEINTERFACE)
		defer C.SetupDiDestroyDeviceInfoList(deviceInfoSet)

		var deviceIdx C.DWORD = 0
		var deviceInterfaceData C.SP_DEVICE_INTERFACE_DATA
		deviceInterfaceData.cbSize = C.DWORD(unsafe.Sizeof(deviceInterfaceData))

		for ; ; deviceIdx++ {
			res := C.SetupDiEnumDeviceInterfaces(deviceInfoSet, nil, &InterfaceClassGuid, deviceIdx, &deviceInterfaceData)
			if res == 0 {
				break
			}
			di := getDeviceDetails(deviceInfoSet, &deviceInterfaceData)
			if di != nil {
				result <- di
			}
		}
	}()
	return result
}

// Open opens the device for read / write access.
func (di *DeviceInfo) Open() (Device, error) {
	d, err := openDevice(di, false)
	if err != nil {
		return nil, err
	}
	if d.isValid() {
		return d, nil
	} else {
		d.Close()
		err := syscall.GetLastError()
		if err == nil {
			err = errors.New("hid: unable to open device")
		}
		return nil, err
	}
}

// Supported returns whether this platform is supported by the HID library or not.
// The goal of this method is to allow programmatically handling platforms that do
// not support USB HID and not having to fall back to build constraints.
func Supported() bool {
	return true
}
