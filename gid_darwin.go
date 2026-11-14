//go:build darwin && cgo && !ios
// +build darwin,cgo,!ios

package gid

/*
#cgo LDFLAGS: -L . -L/usr/local/lib -framework CoreFoundation -framework IOKit

#include <IOKit/hid/IOHIDManager.h>
#include <CoreFoundation/CoreFoundation.h>

// Nonzero matching numbers are retained by the dictionary, then temporary
// references are released. Legacy metadata maps missing/non-number IDs to 0,
// whereas IOKit numeric 0 does not match an absent property. Leave zero fields
// to the final Go equality/path check. NULL is allocation failure, not wildcard.
static CFMutableDictionaryRef gid_device_matching(uint16_t vendor, uint16_t product) {
    CFMutableDictionaryRef matching = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    if (!matching) return NULL;
    if (vendor != 0) {
        int32_t value = vendor;
        CFNumberRef number = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &value);
        if (!number) { CFRelease(matching); return NULL; }
        CFDictionarySetValue(matching, CFSTR(kIOHIDVendorIDKey), number);
        CFRelease(number);
    }
    if (product != 0) {
        int32_t value = product;
        CFNumberRef number = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &value);
        if (!number) { CFRelease(matching); return NULL; }
        CFDictionarySetValue(matching, CFSTR(kIOHIDProductIDKey), number);
        CFRelease(number);
    }
    return matching;
}

// Read only CoreFoundation data. This lets software tests inspect the actual
// native matching dictionary without creating a manager or enumerating HID.
static int gid_matching_values(CFDictionaryRef matching, int32_t *vendor, int32_t *product, uint8_t *present) {
    if (!matching) return 0;
    CFTypeRef v = CFDictionaryGetValue(matching, CFSTR(kIOHIDVendorIDKey));
    CFTypeRef p = CFDictionaryGetValue(matching, CFSTR(kIOHIDProductIDKey));
    *vendor = 0; *product = 0; *present = 0;
    if (v) {
        if (CFGetTypeID(v) != CFNumberGetTypeID() || !CFNumberGetValue((CFNumberRef)v, kCFNumberSInt32Type, vendor)) return 0;
        *present |= 1;
    }
    if (p) {
        if (CFGetTypeID(p) != CFNumberGetTypeID() || !CFNumberGetValue((CFNumberRef)p, kCFNumberSInt32Type, product)) return 0;
        *present |= 2;
    }
    return CFDictionaryGetCount(matching) == ((v ? 1 : 0) + (p ? 1 : 0));
}

// Preserve getIntProp's historical normalization, independently testable with
// CF data only. A missing or non-number property becomes metadata ID zero.
static int32_t gid_integer_property(CFTypeRef property) {
    int32_t value = 0;
    if (property && CFGetTypeID(property) == CFNumberGetTypeID()) {
        CFNumberGetValue((CFNumberRef)property, kCFNumberSInt32Type, &value);
    }
    return value;
}

// Pure-CF test fixtures: kind 0=missing, 1=non-number, 2=numeric zero,
// 3=numeric value. No manager, service lookup or native HID handle is involved.
static CFMutableDictionaryRef gid_property_fixture(int vendor_kind, int product_kind, int32_t vendor, int32_t product) {
    CFMutableDictionaryRef properties = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    if (!properties) return NULL;
    const CFStringRef keys[] = { CFSTR(kIOHIDVendorIDKey), CFSTR(kIOHIDProductIDKey) };
    int kinds[] = {vendor_kind, product_kind};
    int32_t values[] = {vendor, product};
    for (int i=0; i<2; i++) {
        if (kinds[i] == 0) continue;
        if (kinds[i] == 1) {
            CFDictionarySetValue(properties, keys[i], CFSTR("not a numeric HID ID"));
        } else {
            int32_t value = kinds[i] == 2 ? 0 : values[i];
            CFNumberRef number = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &value);
            if (!number) { CFRelease(properties); return NULL; }
            CFDictionarySetValue(properties, keys[i], number);
            CFRelease(number);
        }
    }
    return properties;
}

static void gid_property_fixture_ids(CFDictionaryRef properties, int32_t *vendor, int32_t *product) {
    *vendor = gid_integer_property(CFDictionaryGetValue(properties, CFSTR(kIOHIDVendorIDKey)));
    *product = gid_integer_property(CFDictionaryGetValue(properties, CFSTR(kIOHIDProductIDKey)));
}

static CFTypeRef gid_property_fixture_vendor(CFDictionaryRef properties) {
    return CFDictionaryGetValue(properties, CFSTR(kIOHIDVendorIDKey));
}

// Software model of Apple's CompareProperty: an omitted condition is allowed;
// a present numeric condition cannot equal a missing/non-number property.
static int gid_property_fixture_matches(CFDictionaryRef matching, CFDictionaryRef properties) {
    if (!matching || !properties) return 0;
    const CFStringRef keys[] = { CFSTR(kIOHIDVendorIDKey), CFSTR(kIOHIDProductIDKey) };
    for (int i=0; i<2; i++) {
        CFTypeRef expected = CFDictionaryGetValue(matching, keys[i]);
        if (expected) {
            CFTypeRef actual = CFDictionaryGetValue(properties, keys[i]);
            if (!actual || !CFEqual(expected, actual)) return 0;
        }
    }
    return 1;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

func ioReturnToErr(ret C.IOReturn) error {
	return darwinIOError(int32(ret))
}

// The C boundary retains the signed IOReturn representation. Exposing the low
// 32 bits avoids sign extension when an application records the native code.
func darwinIOError(code int32) error {
	cause := ioReturnCause(C.IOReturn(code))
	if cause == nil {
		return nil
	}
	return &NativeError{Platform: "darwin", Code: uint64(uint32(code)), Err: cause}
}

func ioReturnCause(ret C.IOReturn) error {
	switch ret {
	case C.kIOReturnSuccess:
		return nil
	case C.kIOReturnError:
		return errors.New("general error")
	case C.kIOReturnNoMemory:
		return errors.New("can't allocate memory")
	case C.kIOReturnNoResources:
		return errors.New("resource shortage")
	case C.kIOReturnIPCError:
		return errors.New("error during IPC")
	case C.kIOReturnNoDevice:
		return errors.New("no such device")
	case C.kIOReturnNotPrivileged:
		return errors.New("privilege violation")
	case C.kIOReturnBadArgument:
		return errors.New("invalid argument")
	case C.kIOReturnLockedRead:
		return errors.New("device read locked")
	case C.kIOReturnLockedWrite:
		return errors.New("device write locked")
	case C.kIOReturnExclusiveAccess:
		return errors.New("exclusive access and device already open")
	case C.kIOReturnBadMessageID:
		return errors.New("sent/received messages had different msg_id")
	case C.kIOReturnUnsupported:
		return errors.New("unsupported function")
	case C.kIOReturnVMError:
		return errors.New("misc. VM failure")
	case C.kIOReturnInternalError:
		return errors.New("internal error")
	case C.kIOReturnIOError:
		return errors.New("general I/O error")
	case C.kIOReturnCannotLock:
		return errors.New("can't acquire lock")
	case C.kIOReturnNotOpen:
		return errors.New("device not open")
	case C.kIOReturnNotReadable:
		return errors.New("read not supported")
	case C.kIOReturnNotWritable:
		return errors.New("write not supported")
	case C.kIOReturnNotAligned:
		return errors.New("alignment error")
	case C.kIOReturnBadMedia:
		return errors.New("media error")
	case C.kIOReturnStillOpen:
		return errors.New("device(s) still open")
	case C.kIOReturnRLDError:
		return errors.New("rld failure")
	case C.kIOReturnDMAError:
		return errors.New("DMA failure")
	case C.kIOReturnBusy:
		return errors.New("device is busy")
	case C.kIOReturnTimeout:
		return errors.New("I/O Timeout")
	case C.kIOReturnOffline:
		return errors.New("device offline")
	case C.kIOReturnNotReady:
		return errors.New("not ready")
	case C.kIOReturnNotAttached:
		return errors.New("device not attached")
	case C.kIOReturnNoChannels:
		return errors.New("no DMA channels left")
	case C.kIOReturnNoSpace:
		return errors.New("no space for data")
	case C.kIOReturnPortExists:
		return errors.New("port already exists")
	case C.kIOReturnCannotWire:
		return errors.New("can't wire down physical memory")
	case C.kIOReturnNoInterrupt:
		return errors.New("no interrupt attached")
	case C.kIOReturnNoFrames:
		return errors.New("no DMA frames enqueued")
	case C.kIOReturnMessageTooLarge:
		return errors.New("oversize msg received on interrupt port")
	case C.kIOReturnNotPermitted:
		return errors.New("not permitted")
	case C.kIOReturnNoPower:
		return errors.New("no power to device")
	case C.kIOReturnNoMedia:
		return errors.New("media not present")
	case C.kIOReturnUnformattedMedia:
		return errors.New("media not formatted")
	case C.kIOReturnUnsupportedMode:
		return errors.New("no such mode")
	case C.kIOReturnUnderrun:
		return errors.New("data underrun")
	case C.kIOReturnOverrun:
		return errors.New("data overrun")
	case C.kIOReturnDeviceError:
		return errors.New("the device is not working properly")
	case C.kIOReturnNoCompletion:
		return errors.New("a completion routine is required")
	case C.kIOReturnAborted:
		return errors.New("operation aborted")
	case C.kIOReturnNoBandwidth:
		return errors.New("bus bandwidth would be exceeded")
	case C.kIOReturnNotResponding:
		return errors.New("device not responding")
	case C.kIOReturnIsoTooOld:
		return errors.New("isochronous I/O request for distant past")
	case C.kIOReturnIsoTooNew:
		return errors.New("isochronous I/O request for distant future")
	case C.kIOReturnNotFound:
		return errors.New("data was not found")
	}
	return errors.New("unknown error")
}

type cleanupDeviceManagerFn func()
type osxDevice struct {
	// mu protects the native handle for the entire duration of each synchronous
	// I/O call, including Close. Releasing it before calling IOKit would allow
	// another goroutine to close and release the handle while it is in use.
	mu       sync.Mutex
	osDevice C.IOHIDDeviceRef
	closeDM  cleanupDeviceManagerFn
}

func cfstring(s string) C.CFStringRef {
	buf := []byte(s)
	var data *C.UInt8
	if len(buf) > 0 {
		data = (*C.UInt8)(unsafe.Pointer(&buf[0]))
	}
	return C.CFStringCreateWithBytes(0, data, C.CFIndex(len(buf)), C.kCFStringEncodingUTF8, 0)
}

func releaseCFString(s C.CFStringRef) {
	if s != 0 {
		C.CFRelease(C.CFTypeRef(s))
	}
}

func gostring(cfs C.CFStringRef) string {
	// IOHIDDeviceGetProperty returns an untyped CF object. Optional metadata
	// must be a string before calling CFString APIs: a numeric property can
	// otherwise raise an Objective-C exception and abort the process.
	if cfs == 0 || C.CFGetTypeID(C.CFTypeRef(cfs)) != C.CFStringGetTypeID() {
		return ""
	}

	rng := C.CFRange{location: 0, length: C.CFStringGetLength(cfs)}
	var usedBufLen C.CFIndex
	if C.CFStringGetBytes(cfs, rng, C.kCFStringEncodingUTF8, 0, 0, nil, 0, &usedBufLen) != rng.length || usedBufLen == 0 {
		return ""
	}
	buf := make([]byte, int(usedBufLen))
	if C.CFStringGetBytes(cfs, rng, C.kCFStringEncodingUTF8, 0, 0, (*C.UInt8)(unsafe.Pointer(&buf[0])), C.CFIndex(len(buf)), &usedBufLen) != rng.length {
		return ""
	}
	return string(buf[:int(usedBufLen)])
}

func getIntProp(device C.IOHIDDeviceRef, key string) int32 {
	cfKey := cfstring(key)
	defer releaseCFString(cfKey)
	ref := C.IOHIDDeviceGetProperty(device, cfKey)
	return int32(C.gid_integer_property(ref))
}

func getStringProp(device C.IOHIDDeviceRef, key string) string {
	cfKey := cfstring(key)
	defer releaseCFString(cfKey)
	s := C.IOHIDDeviceGetProperty(device, cfKey)
	return gostring(C.CFStringRef(s))
}

func getPath(osDev C.IOHIDDeviceRef) string {
	return fmt.Sprintf("%s_%04x_%04x_%08x",
		getStringProp(osDev, C.kIOHIDTransportKey),
		uint16(getIntProp(osDev, C.kIOHIDVendorIDKey)),
		uint16(getIntProp(osDev, C.kIOHIDProductIDKey)),
		uint32(getIntProp(osDev, C.kIOHIDLocationIDKey)))
}

type darwinMatchingDictionary struct{ ref C.CFMutableDictionaryRef }

func (d *darwinMatchingDictionary) close() {
	if d.ref != 0 {
		C.CFRelease(C.CFTypeRef(d.ref))
		d.ref = 0
	}
}

func (d darwinMatchingDictionary) values() (int32, int32, uint8, bool) {
	var vendor, product C.int32_t
	var present C.uint8_t
	ok := C.gid_matching_values(C.CFDictionaryRef(d.ref), &vendor, &product, &present)
	return int32(vendor), int32(product), uint8(present), ok != 0
}

func darwinPropertyFixture(vendorKind, productKind int, vendor, product int32) darwinMatchingDictionary {
	return darwinMatchingDictionary{ref: C.gid_property_fixture(C.int(vendorKind), C.int(productKind), C.int32_t(vendor), C.int32_t(product))}
}

func (d darwinMatchingDictionary) normalizedIDs() darwinDeviceIDs {
	var vendor, product C.int32_t
	C.gid_property_fixture_ids(C.CFDictionaryRef(d.ref), &vendor, &product)
	return darwinDeviceIDs{vendor: uint16(vendor), product: uint16(product)}
}

// fixtureVendorString reads existing pure-CF metadata fixtures without creating
// a HID manager, enumerating devices, or opening a native device.
func (d darwinMatchingDictionary) fixtureVendorString() string {
	property := C.gid_property_fixture_vendor(C.CFDictionaryRef(d.ref))
	return gostring(C.CFStringRef(property))
}

func (d darwinMatchingDictionary) acceptsFixture(properties darwinMatchingDictionary) bool {
	return C.gid_property_fixture_matches(C.CFDictionaryRef(d.ref), C.CFDictionaryRef(properties.ref)) != 0
}

func matchingDictionary(ids *darwinDeviceIDs, create func(darwinDeviceIDs) darwinMatchingDictionary) (darwinMatchingDictionary, error) {
	// A nil filter is reserved for explicit all-metadata enumeration.
	if ids == nil {
		return darwinMatchingDictionary{}, nil
	}
	dictionary := create(*ids)
	if dictionary.ref == 0 {
		return dictionary, fmt.Errorf("hid: allocate device matching: %w", ioReturnToErr(C.kIOReturnNoMemory))
	}
	return dictionary, nil
}

func createMatchingDictionary(ids darwinDeviceIDs) darwinMatchingDictionary {
	return darwinMatchingDictionary{ref: C.gid_device_matching(C.uint16_t(ids.vendor), C.uint16_t(ids.product))}
}

// Creating/matching/copying a manager is metadata discovery, not device open.
// IOHIDManagerOpen would open every matched HID (historically every HID on the
// host) before the caller's filter could run, potentially involving unrelated
// keyboards and Input Monitoring consent. Only DeviceInfo.Open opens a device.
func iterateDevices(ids *darwinDeviceIDs, action func(device C.IOHIDDeviceRef) bool) (cleanupDeviceManagerFn, error) {
	dictionary, err := matchingDictionary(ids, createMatchingDictionary)
	if err != nil {
		return func() {}, err
	}
	defer dictionary.close()
	mgr := C.IOHIDManagerCreate(C.kCFAllocatorDefault, C.kIOHIDOptionsTypeNone)
	if mgr == 0 {
		return func() {}, ioReturnToErr(C.kIOReturnNoMemory)
	}
	cleanup := func() {
		C.CFRelease(C.CFTypeRef(mgr))
	}
	C.IOHIDManagerSetDeviceMatching(mgr, C.CFDictionaryRef(dictionary.ref))

	allDevicesSet := C.IOHIDManagerCopyDevices(mgr)
	if allDevicesSet == 0 {
		return cleanup, nil
	}
	defer C.CFRelease(C.CFTypeRef(allDevicesSet))
	devCnt := C.CFSetGetCount(allDevicesSet)
	if devCnt == 0 {
		return cleanup, nil
	}
	allDevices := make([]unsafe.Pointer, uint64(devCnt))
	C.CFSetGetValues(allDevicesSet, &allDevices[0])

	for _, pDev := range allDevices {
		if !action(C.IOHIDDeviceRef(pDev)) {
			break
		}
	}
	return cleanup, nil
}

// Devices returns a channel that will receive a DeviceInfo struct for each HID device.
func Devices() <-chan *DeviceInfo {
	return darwinDevices(nil)
}

func devicesByID(vendor, product uint16) <-chan *DeviceInfo {
	return darwinDevices(&darwinDeviceIDs{vendor: vendor, product: product})
}

func darwinDevices(ids *darwinDeviceIDs) <-chan *DeviceInfo {
	result := make(chan *DeviceInfo, maxDeviceChannelSize)
	go func() {
		defer close(result)

		// Finish enumeration before sending results. A caller may Open a
		// received device while the channel is full; Open needs the same lock.
		var infos []*DeviceInfo
		func() {
			enumerateLock.Lock()
			defer enumerateLock.Unlock()
			cleanup, _ := iterateDevices(ids, func(device C.IOHIDDeviceRef) bool {
				infos = append(infos, &DeviceInfo{
					VendorID:            uint16(getIntProp(device, C.kIOHIDVendorIDKey)),
					ProductID:           uint16(getIntProp(device, C.kIOHIDProductIDKey)),
					VersionNumber:       uint16(getIntProp(device, C.kIOHIDVersionNumberKey)),
					Manufacturer:        getStringProp(device, C.kIOHIDManufacturerKey),
					Product:             getStringProp(device, C.kIOHIDProductKey),
					SerialNumber:        getStringProp(device, C.kIOHIDSerialNumberKey),
					InputReportLength:   uint16(getIntProp(device, C.kIOHIDMaxInputReportSizeKey)),
					OutputReportLength:  uint16(getIntProp(device, C.kIOHIDMaxOutputReportSizeKey)),
					FeatureReportLength: uint16(getIntProp(device, C.kIOHIDMaxFeatureReportSizeKey)),
					Path:                getPath(device),
				})
				return true
			})
			cleanup()
		}()
		for _, info := range infos {
			result <- info
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

// Open opens the device for reading and writing.
func (di *DeviceInfo) Open() (Device, error) {
	ids, matchErr := darwinOpenIDs(di)
	if matchErr != nil {
		return nil, matchErr
	}
	enumerateLock.Lock()
	defer enumerateLock.Unlock()

	err := errDeviceNotFound
	var dev *osxDevice
	closeDM, enumerateErr := iterateDevices(&ids, func(device C.IOHIDDeviceRef) bool {
		if getPath(device) == di.Path {
			res := C.IOHIDDeviceOpen(device, C.kIOHIDOptionsTypeSeizeDevice)
			if res == C.kIOReturnSuccess {
				C.CFRetain(C.CFTypeRef(device))
				dev = &osxDevice{osDevice: device}
				err = nil
				// This backend uses synchronous I/O and has no scheduled run loop.
				// Let those calls report removal errors instead of registering an
				// asynchronous callback that retains a Go pointer in IOKit.
				// Callers must still Close the device to release its resources.
			} else {
				err = ioReturnToErr(res)
			}
			return false
		}
		return true
	})
	if enumerateErr != nil {
		closeDM()
		return nil, enumerateErr
	}
	if dev == nil {
		closeDM()
		return nil, err
	}
	dev.closeDM = closeDM
	return dev, err
}

func (dev *osxDevice) Close() {
	dev.mu.Lock()
	defer dev.mu.Unlock()

	if dev.osDevice != 0 {
		C.IOHIDDeviceClose(dev.osDevice, C.kIOHIDOptionsTypeSeizeDevice)
		C.CFRelease(C.CFTypeRef(dev.osDevice))
		dev.osDevice = 0
	}
	if dev.closeDM != nil {
		dev.closeDM()
		dev.closeDM = nil
	}
}

func (dev *osxDevice) setReport(typ C.IOHIDReportType, data []byte) error {
	dev.mu.Lock()
	defer dev.mu.Unlock()
	if dev.osDevice == 0 {
		return errDeviceClosed
	}
	if err := validateReportBuffer(data); err != nil {
		return err
	}

	var reportNo = int32(data[0])
	if reportNo == 0 {
		data = data[1:]
	}

	res := C.IOHIDDeviceSetReport(dev.osDevice, typ, C.CFIndex(reportNo), (*C.uint8_t)(&data[0]), C.CFIndex(len(data)))

	if res == C.kIOReturnSuccess {
		return nil
	}
	return ioReturnToErr(res)
}

func (dev *osxDevice) getReport(typ C.IOHIDReportType, buf []byte) (int, error) {
	dev.mu.Lock()
	defer dev.mu.Unlock()
	if dev.osDevice == 0 {
		return 0, errDeviceClosed
	}
	if err := validateReportBuffer(buf); err != nil {
		return 0, err
	}

	var (
		reportNo  = int32(buf[0])
		reportLen = C.CFIndex(len(buf))
	)
	if reportNo == 0 {
		/* Not using numbered Reports. Don't send the report number. */
		buf = buf[1:]
		reportLen--
	}

	res := C.IOHIDDeviceGetReport(dev.osDevice, typ, C.CFIndex(reportNo), (*C.uint8_t)(&buf[0]), &reportLen)
	if res != C.kIOReturnSuccess {
		return 0, ioReturnToErr(res)
	}

	if reportNo == 0 {
		/* 0 report number still present at the beginning */
		reportLen++
	}
	return int(reportLen), nil
}

func (dev *osxDevice) WriteFeature(data []byte) error {
	return dev.setReport(C.kIOHIDReportTypeFeature, data)
}

func (dev *osxDevice) Write(data []byte) error {
	return dev.setReport(C.kIOHIDReportTypeOutput, data)
}

func (dev *osxDevice) WriteInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

func (dev *osxDevice) ReadFeature(buf []byte) (int, error) {
	return dev.getReport(C.kIOHIDReportTypeFeature, buf)
}

func (dev *osxDevice) Read(buf []byte) (int, error) {
	return dev.getReport(C.kIOHIDReportTypeInput, buf)
}

func (dev *osxDevice) ReadInterrupt(endpoint byte, data []byte) (int, error) {
	return 0, errNotImplemented
}

// Supported returns whether this platform is supported by the HID library or not.
// The goal of this method is to allow programmatically handling platforms that do
// not support USB HID and not having to fall back to build constraints.
func Supported() bool {
	return true
}
