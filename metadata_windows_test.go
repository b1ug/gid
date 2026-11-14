//go:build windows && cgo
// +build windows,cgo

package gid

import "testing"

func TestWindowsHIDMetadataBufferLengthUsesNativeBytes(t *testing.T) {
	if got := windowsHIDStringBufferBytes(make([]uint16, windowsHIDStringCodeUnits)); got != 512 {
		t.Fatalf("native UTF-16 buffer byte length = %d, want 512", got)
	}
	if got := windowsHIDStringBufferBytes(nil); got != 0 {
		t.Fatalf("empty native buffer byte length = %d, want 0", got)
	}
}

func TestWindowsHIDAttributesRejectInvalidHandle(t *testing.T) {
	vendor, product, version, err := getWindowsHIDAttributes(0)
	if err == nil || vendor != 0 || product != 0 || version != 0 {
		t.Fatalf("invalid handle attributes = %#x/%#x/%#x, %v", vendor, product, version, err)
	}
}
