package gid

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWindowsCStringCopiesOnlySuccessfulBoundedResults(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		offset      int
	}{
		{"property", "HIDClass", 0},
		{"device path", `\\?\hid#vid_27b8&pid_01ed#serial`, 4},
		{"empty property", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			size := tc.offset + len(tc.value) + 1
			got := getCString(func(buffer []byte) (int, uint32, uint32) {
				calls++
				if buffer == nil {
					return tc.offset, uint32(size), windowsStringInsufficientBuffer
				}
				if len(buffer) != size || !bytes.Equal(buffer, make([]byte, size)) {
					t.Fatal("fill buffer must be sized and zero initialized")
				}
				if tc.offset > 0 {
					binary.LittleEndian.PutUint32(buffer, 5)
				}
				copy(buffer[tc.offset:], tc.value)
				return tc.offset, uint32(size), windowsStringSuccess
			})
			if got != tc.value || calls != 2 {
				t.Fatalf("query = %q in %d calls, want %q in two calls", got, calls, tc.value)
			}
		})
	}
}

func TestWindowsCStringRejectsFailedProbesWithoutAllocatingOrFilling(t *testing.T) {
	for _, tc := range []struct {
		name       string
		size, code uint32
	}{
		{"probe failed", 16, 5},
		{"empty required size", 0, windowsStringInsufficientBuffer},
		{"unbounded required size", maxWindowsStringBytes + 1, windowsStringInsufficientBuffer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got := getCString(func(buffer []byte) (int, uint32, uint32) {
				calls++
				if buffer != nil {
					t.Fatal("failed size probe reached fill")
				}
				return 0, tc.size, tc.code
			})
			if got != "" || calls != 1 {
				t.Fatalf("query=%q, calls=%d", got, calls)
			}
		})
	}
}

func TestWindowsCStringRejectsFailedInconsistentAndUnterminatedFills(t *testing.T) {
	for _, tc := range []struct {
		name       string
		offset     int
		used, code uint32
		data       string
	}{
		{"failed fill with plausible data", 0, 8, 1167, "stale\x00"},
		{"native size grew", 0, 9, windowsStringSuccess, "good\x00"},
		{"zero returned size", 0, 0, windowsStringSuccess, "good\x00"},
		{"negative string offset", -1, 8, windowsStringSuccess, "good\x00"},
		{"string outside buffer", 8, 8, windowsStringSuccess, "good\x00"},
		{"string outside returned bytes", 4, 3, windowsStringSuccess, "good\x00"},
		{"unterminated allocation", 0, 8, windowsStringSuccess, "ABCDEFGH"},
		{"terminator beyond returned data", 0, 3, windowsStringSuccess, "ABC\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got := getCString(func(buffer []byte) (int, uint32, uint32) {
				calls++
				if buffer == nil {
					return 0, 8, windowsStringInsufficientBuffer
				}
				copy(buffer, tc.data)
				return tc.offset, tc.used, tc.code
			})
			if got != "" || calls != 2 {
				t.Fatalf("untrusted fill returned %q in %d calls", got, calls)
			}
		})
	}
}

func TestWindowsCStringAcceptsTerminatorAtLastAllocatedByte(t *testing.T) {
	got := getCString(func(buffer []byte) (int, uint32, uint32) {
		if buffer == nil {
			return 0, 6, windowsStringInsufficientBuffer
		}
		copy(buffer, "valid\x00")
		return 0, 6, windowsStringSuccess
	})
	if got != "valid" {
		t.Fatalf("last-byte terminator not honored: %q", got)
	}
}
