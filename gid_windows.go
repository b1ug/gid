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

// Windows handles are opaque integer values, not Go pointers. Keep the
// integer-to-HANDLE conversion in C so Go's GC never sees a fake pointer.
static BOOL gid_get_overlapped_result(uintptr_t handle, LPOVERLAPPED overlapped, LPDWORD transferred, BOOL wait) {
    return GetOverlappedResult((HANDLE)handle, overlapped, transferred, wait);
}

static BOOLEAN gid_set_feature(uintptr_t handle, PVOID buffer, ULONG length) {
    return HidD_SetFeature((HANDLE)handle, buffer, length);
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
	handle syscall.Handle
	info   *DeviceInfo
}

// checks if the handle of the device is valid
func (dev *winDevice) isValid() bool {
	return dev.handle != 0 && dev.handle != syscall.InvalidHandle
}

func (dev *winDevice) Close() {
	if dev.isValid() {
		syscall.CloseHandle(dev.handle)
		dev.handle = syscall.InvalidHandle
	}
}

func (dev *winDevice) Write(data []byte) error {
	if !dev.isValid() {
		return errDeviceClosed
	}
	// Windows requires the collection's complete output report size.
	outSize := int(dev.info.OutputReportLength)
	buffer, err := prepareWriteReport(data, outSize)
	if err != nil {
		return err
	}

	ol := new(syscall.Overlapped)
	if err := syscall.WriteFile(dev.handle, buffer, nil, ol); err != nil {
		// IO Pending is ok we simply wait for it to finish a few lines below
		// all other errors should be reported.
		if !errors.Is(err, syscall.ERROR_IO_PENDING) {
			return err
		}
	}

	// now wait for the overlapped device access to finish.
	var written C.DWORD
	if C.gid_get_overlapped_result(C.uintptr_t(dev.handle), (*C.OVERLAPPED)((unsafe.Pointer)(ol)), &written, C.TRUE) == 0 {
		return syscall.GetLastError()
	}

	if int(written) != outSize {
		return errors.New("hid: written bytes mismatch")
	}
	return nil
}

func (dev *winDevice) WriteFeature(data []byte) error {
	if !dev.isValid() {
		return errDeviceClosed
	}
	buffer, err := prepareWriteReport(data, int(dev.info.FeatureReportLength))
	if err != nil {
		return err
	}

	// Reference: https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidsdi/nf-hidsdi-hidd_setfeature
	cBuffer := (C.PVOID)(unsafe.Pointer(&buffer[0]))
	if C.gid_set_feature(C.uintptr_t(dev.handle), cBuffer, C.ULONG(len(buffer))) != 0 {
		return nil
	} else {
		return fmt.Errorf("hid: set feature fail: %w", syscall.GetLastError())
	}
}

func (dev *winDevice) WriteInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

func (dev *winDevice) Read(data []byte) (int, error) {
	if !dev.isValid() {
		return 0, errDeviceClosed
	}
	// Reference: https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidclass/ni-hidclass-ioctl_hid_get_input_report
	return dev.readReport(data, ioctlHidGetInputReport, int(dev.info.InputReportLength))
}

func (dev *winDevice) ReadFeature(data []byte) (int, error) {
	if !dev.isValid() {
		return 0, errDeviceClosed
	}
	// Reference: https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidclass/ni-hidclass-ioctl_hid_get_feature
	return dev.readReport(data, ioctlHidGetFeature, int(dev.info.FeatureReportLength))
}

func (dev *winDevice) ReadInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

func (dev *winDevice) readReport(data []byte, reportType uint32, minBufSize int) (int, error) {
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

	// call the Windows API
	var (
		ol            = new(C.OVERLAPPED)
		bytesReturned uint32
	)
	if err := syscall.DeviceIoControl(
		dev.handle,
		reportType,
		&buf[0],
		uint32(len(buf)),
		&buf[0],
		uint32(len(buf)),
		&bytesReturned,
		(*syscall.Overlapped)(unsafe.Pointer(ol)),
	); err != nil {
		// IO Pending is ok we simply wait for it to finish, a few lines below all other errors should be reported.
		if !errors.Is(err, syscall.ERROR_IO_PENDING) {
			return 0, fmt.Errorf("hid: send device control fail: %w", err)
		}
	}

	// now wait for the overlapped device access to finish.
	var read C.DWORD
	if C.gid_get_overlapped_result(C.uintptr_t(dev.handle), (*C.OVERLAPPED)((unsafe.Pointer)(ol)), &read, C.TRUE) == 0 {
		return 0, fmt.Errorf("hid: retrieve result fail: %w", syscall.GetLastError())
	}

	if reportType == ioctlHidGetInputReport {
		return copyInputReport(data, buf, int(read))
	}
	return copyReadReport(data, buf, int(read))
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
