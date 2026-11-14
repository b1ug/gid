//go:build cgo
// +build cgo

package gid

import (
	"syscall"
	"testing"
)

func TestWindowsClosedDeviceReports(t *testing.T) {
	// Neither value names a native handle, so no attached HID device is opened.
	for _, handle := range []syscall.Handle{0, syscall.InvalidHandle} {
		dev := &winDevice{handle: handle}
		dev.Close()
		dev.Close()
		for _, data := range [][]byte{nil, {0}, {1}, {0, 42}, {1, 42}} {
			for name, write := range map[string]func([]byte) error{
				"Write": dev.Write, "WriteFeature": dev.WriteFeature,
			} {
				if err := write(data); err != errDeviceClosed {
					t.Errorf("%s(%v) = %v, want closed error", name, data, err)
				}
			}
			for name, read := range map[string]func([]byte) (int, error){
				"Read": dev.Read, "ReadFeature": dev.ReadFeature,
			} {
				if n, err := read(data); n != 0 || err != errDeviceClosed {
					t.Errorf("%s(%v) = %d, %v; want 0, closed error", name, data, n, err)
				}
			}
		}
	}
}
