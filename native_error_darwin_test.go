//go:build darwin && cgo
// +build darwin,cgo

package gid

import (
	"errors"
	"fmt"
	"testing"
)

func TestDarwinNativeErrorPreservesMessagesAndCodes(t *testing.T) {
	// Freeze the pre-existing messages against the actual IOKit code values.
	// No native device is enumerated or opened by these conversion tests.
	for _, test := range []struct {
		code uint32
		text string
	}{
		{0xe00002bc, "general error"},
		{0xe00002bd, "can't allocate memory"},
		{0xe00002be, "resource shortage"},
		{0xe00002bf, "error during IPC"},
		{0xe00002c0, "no such device"},
		{0xe00002c1, "privilege violation"},
		{0xe00002c2, "invalid argument"},
		{0xe00002c3, "device read locked"},
		{0xe00002c4, "device write locked"},
		{0xe00002c5, "exclusive access and device already open"},
		{0xe00002c6, "sent/received messages had different msg_id"},
		{0xe00002c7, "unsupported function"},
		{0xe00002c8, "misc. VM failure"},
		{0xe00002c9, "internal error"},
		{0xe00002ca, "general I/O error"},
		{0xe00002cc, "can't acquire lock"},
		{0xe00002cd, "device not open"},
		{0xe00002ce, "read not supported"},
		{0xe00002cf, "write not supported"},
		{0xe00002d0, "alignment error"},
		{0xe00002d1, "media error"},
		{0xe00002d2, "device(s) still open"},
		{0xe00002d3, "rld failure"},
		{0xe00002d4, "DMA failure"},
		{0xe00002d5, "device is busy"},
		{0xe00002d6, "I/O Timeout"},
		{0xe00002d7, "device offline"},
		{0xe00002d8, "not ready"},
		{0xe00002d9, "device not attached"},
		{0xe00002da, "no DMA channels left"},
		{0xe00002db, "no space for data"},
		{0xe00002dd, "port already exists"},
		{0xe00002de, "can't wire down physical memory"},
		{0xe00002df, "no interrupt attached"},
		{0xe00002e0, "no DMA frames enqueued"},
		{0xe00002e1, "oversize msg received on interrupt port"},
		{0xe00002e2, "not permitted"},
		{0xe00002e3, "no power to device"},
		{0xe00002e4, "media not present"},
		{0xe00002e5, "media not formatted"},
		{0xe00002e6, "no such mode"},
		{0xe00002e7, "data underrun"},
		{0xe00002e8, "data overrun"},
		{0xe00002e9, "the device is not working properly"},
		{0xe00002ea, "a completion routine is required"},
		{0xe00002eb, "operation aborted"},
		{0xe00002ec, "bus bandwidth would be exceeded"},
		{0xe00002ed, "device not responding"},
		{0xe00002ee, "isochronous I/O request for distant past"},
		{0xe00002ef, "isochronous I/O request for distant future"},
		{0xe00002f0, "data was not found"},
		{0xe0000001, "unknown error"},
		{0x12345678, "unknown error"},
		{0xffffffff, "unknown error"},
	} {
		t.Run(fmt.Sprintf("%08x", test.code), func(t *testing.T) {
			err := darwinIOError(int32(test.code))
			if err == nil || err.Error() != test.text {
				t.Fatalf("message = %v, want %q", err, test.text)
			}
			var details interface {
				NativeCode() uint64
				NativePlatform() string
			}
			if !errors.As(fmt.Errorf("wrapped: %w", err), &details) || details.NativeCode() != uint64(test.code) || details.NativePlatform() != "darwin" {
				t.Fatalf("native details were not preserved: %#v", details)
			}
			if cause := errors.Unwrap(err); cause == nil || cause.Error() != test.text || !errors.Is(err, cause) {
				t.Fatalf("legacy cause was not preserved: %v", cause)
			}
		})
	}
}

func TestDarwinNativeSuccessIsNil(t *testing.T) {
	if err := darwinIOError(0); err != nil {
		t.Fatalf("success returned a non-nil error interface: %v", err)
	}
}
