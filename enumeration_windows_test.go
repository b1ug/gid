//go:build windows && cgo
// +build windows,cgo

package gid

import (
	"testing"
	"unsafe"
)

func TestWindowsDetailHeaderMatchesNativeABI(t *testing.T) {
	// This checks header layout only; it does not enumerate or open hardware.
	want := uint32(8)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 5
	}
	if got := windowsDetailHeaderSize(); got != want {
		t.Fatalf("native ANSI detail header size=%d, want%d", got, want)
	}
}
