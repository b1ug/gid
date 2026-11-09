//go:build !darwin || !cgo || ios
// +build !darwin !cgo ios

package gid

func devicesByID(_, _ uint16) <-chan *DeviceInfo { return Devices() }
