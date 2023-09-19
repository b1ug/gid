//go:build (!linux && !darwin && !windows) || ios || !cgo
// +build !linux,!darwin,!windows ios !cgo

package gid

// Devices returns a channel that will receive a DeviceInfo struct for each HID device.
func Devices() <-chan *DeviceInfo {
	result := make(chan *DeviceInfo)
	close(result)
	return result
}

// ByPath returns a device by its path.
func ByPath(path string) (*DeviceInfo, error) {
	return nil, errUnsupportedPlatform
}

// Supported returns whether this platform is supported by the HID library or not.
// The goal of this method is to allow programmatically handling platforms that do
// not support USB HID and not having to fall back to build constraints.
func Supported() bool {
	return false
}
