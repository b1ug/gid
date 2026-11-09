// Package gid provides a platform independent interface to access Human Interface Devices.
// The platform specific parts of this package are heavily based on Signal 11 - HIDAPI (https://github.com/signal11/hidapi).
package gid

import (
	"errors"
	"strings"
	"sync"
)

// DeviceInfo provides general information about a device
type DeviceInfo struct {
	// Path contains a Platform-specific device path which is used to identify the device
	Path string
	// VendorID contains the USB Vendor ID of the device
	VendorID uint16
	// ProductID contains the USB Product ID of the device
	ProductID uint16
	// VersionNumber contains the Version / Release Number of the device
	VersionNumber uint16
	// Manufacturer of the USB device
	Manufacturer string
	// Product contains the product name of the device
	Product string
	// SerialNumber contains the serial number of the device
	SerialNumber string

	InputReportLength   uint16
	OutputReportLength  uint16
	FeatureReportLength uint16
}

// Device interface for an opened HID USB device
type Device interface {
	// Close closes the device and release all kept resources.
	Close()
	// Write sends an output report. The first byte is the report ID, or zero
	// for an unnumbered report. A zero ID must be followed by report data.
	// Empty buffers are rejected.
	Write([]byte) error
	// WriteFeature sends a feature report using the same buffer format as Write.
	WriteFeature([]byte) error
	// Read from the device
	Read([]byte) (int, error)
	// ReadFeature retrieves a feature report. The first byte selects the report
	// ID (zero for unnumbered reports); allow space for this ID in the buffer.
	// Empty buffers and a zero ID without room for data are rejected.
	ReadFeature([]byte) (int, error)
}

// ListFirstDevice returns the first device of which the cond function returns true.
// If cond is nil, the first device is returned.
// If no device is found, nil is returned.
func ListFirstDevice(cond func(*DeviceInfo) bool) *DeviceInfo {
	ch := Devices()
	defer drainDevices(ch)

	for dev := range ch {
		if cond == nil || cond(dev) {
			return dev
		}
	}
	return nil
}

// ListAllDevices returns all devices of which the cond function returns true.
// If cond is nil, all devices are returned.
func ListAllDevices(cond func(*DeviceInfo) bool) []*DeviceInfo {
	var result []*DeviceInfo
	for dev := range Devices() {
		if cond == nil || cond(dev) {
			result = append(result, dev)
		}
	}
	return result
}

// FindDevices creates a channel to emit all devices with a given vendor and product id.
// It returns a channel which is closed when all devices have been enumerated.
// Zero IDs match literally in returned metadata; they are not wildcards.
// Darwin prefilters nonzero IDs before collecting metadata, without opening
// a whole HID manager, then preserves the historical Go equality check.
func FindDevices(vendor uint16, product uint16) <-chan *DeviceInfo {
	return filterDeviceIDs(devicesByID(vendor, product), vendor, product)
}

// Keep exact equality after native matching too: zero is a real ID here,
// never a wildcard. Other backends can retain their existing enumeration.
func filterDeviceIDs(devices <-chan *DeviceInfo, vendor, product uint16) <-chan *DeviceInfo {
	result := make(chan *DeviceInfo)
	go func() {
		defer drainDevices(devices)
		defer close(result)

		for dev := range devices {
			if dev.VendorID == vendor && dev.ProductID == product {
				result <- dev
			}
		}
	}()
	return result
}

// FindDevicesByProduct creates a channel to emit device information where the device's product name contains the given string.
// The channel is closed after all devices have been processed.
func FindDevicesByProduct(product string) <-chan *DeviceInfo {
	result := make(chan *DeviceInfo)

	go func() {
		ch := Devices()
		defer drainDevices(ch)
		defer close(result)

		for dev := range ch {
			if strings.Contains(dev.Product, product) {
				result <- dev
			}
		}
	}()

	return result
}

// drainDevices drains the given channel and discards all values to avoid data leaks.
func drainDevices(ch <-chan *DeviceInfo) {
	go func() {
		// drain the channel
		for range ch {
		}
	}()
}

// enumerateLock is a mutex serializing access to USB device enumeration needed
// by the macOS USB HID system calls, which require 2 consecutive method calls
// for enumeration, causing crashes if called concurrently.
//
// For more details, see:
//
//	https://developer.apple.com/documentation/iokit/1438371-iohidmanagersetdevicematching
//	> "subsequent calls will cause the hid manager to release previously enumerated devices"
var enumerateLock sync.Mutex

var (
	errDeviceNotFound      = errors.New("hid: device not found")
	errDeviceClosed        = errors.New("hid: device disconnected")
	errUnsupportedPlatform = errors.New("hid: unsupported platform")
	errNotImplemented      = errors.New("hid: method not implemented")
)

const maxDeviceChannelSize = 1024
