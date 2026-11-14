//go:build (!darwin && !linux) || !cgo || ios
// +build !darwin,!linux !cgo ios

package gid

func devicesByID(_, _ uint16) <-chan *DeviceInfo { return Devices() }
