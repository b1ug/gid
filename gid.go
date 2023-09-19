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
	// Write to the device (technically a HID report with type 'output' is sent to the device)
	Write([]byte) error
	// WriteFeature to the device (technically a HID report with type 'feature' is sent to the device)
	WriteFeature([]byte) error
	// Read from the device
	Read([]byte) (int, error)
	// ReadFeature from the device
	ReadFeature([]byte) (int, error)
}

// FindDevices iterates through all devices with a given vendor and product id
func FindDevices(vendor uint16, product uint16) <-chan *DeviceInfo {
	result := make(chan *DeviceInfo)
	go func() {
		for dev := range Devices() {
			if dev.VendorID == vendor && dev.ProductID == product {
				result <- dev
			}
		}
		close(result)
	}()
	return result
}

// FindDevicesByProduct iterates through all devices with a given vendor and product id
func FindDevicesByProduct(product string) <-chan *DeviceInfo {
	result := make(chan *DeviceInfo)

	go func() {
		for dev := range Devices() {
			if strings.Contains(dev.Product, product) {
				result <- dev
			}
		}
		close(result)
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
//   https://developer.apple.com/documentation/iokit/1438371-iohidmanagersetdevicematching
//   > "subsequent calls will cause the hid manager to release previously enumerated devices"
var enumerateLock sync.Mutex

var (
	errDeviceNotFound      = errors.New("hid: device not found")
	errDeviceClosed        = errors.New("hid: device disconnected")
	errUnsupportedPlatform = errors.New("hid: unsupported platform")
	errNotImplemented      = errors.New("hid: method not implemented")
)
