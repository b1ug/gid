package gid

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

func windowsMetadataString(value string) func([]uint16) bool {
	return func(buffer []uint16) bool {
		copy(buffer, utf16.Encode([]rune(value+"\x00")))
		return true
	}
}

func TestWindowsMetadataFailuresDoNotReusePreviousFields(t *testing.T) {
	info := &DeviceInfo{Path: "fake-path"}
	calls := 0
	err := populateWindowsHIDMetadata(info, windowsHIDMetadataQueries{
		attributes:   func() (uint16, uint16, uint16, error) { return 0x27b8, 0x01ed, 2, nil },
		manufacturer: windowsMetadataString("ThingM"),
		product: func(buffer []uint16) bool {
			calls++
			if buffer[0] != 0 {
				t.Error("product query reused manufacturer output")
			}
			return false // A failed native call need not replace prior contents.
		},
		serial: func(buffer []uint16) bool {
			calls++
			if buffer[0] != 0 {
				t.Error("serial query did not receive independent zeroed memory")
			}
			copy(buffer, utf16.Encode([]rune("20005601\x00")))
			return false // Partial or otherwise failed output is never identity.
		},
	})
	if err != nil || calls != 2 || info.Manufacturer != "ThingM" || info.Product != "" || info.SerialNumber != "" {
		t.Fatalf("failed metadata queries = %+v, calls=%d, err=%v", info, calls, err)
	}
	if info.VendorID != 0x27b8 || info.ProductID != 0x01ed || info.VersionNumber != 2 || info.Path != "fake-path" {
		t.Fatalf("successful attributes/path changed: %+v", info)
	}
}

func TestWindowsMetadataSerialFailureDoesNotBecomeProduct(t *testing.T) {
	info := &DeviceInfo{}
	err := populateWindowsHIDMetadata(info, windowsHIDMetadataQueries{
		attributes:   func() (uint16, uint16, uint16, error) { return 0x27b8, 0x01ed, 2, nil },
		manufacturer: windowsMetadataString("ThingM"),
		product:      windowsMetadataString("blink(1) mk2"),
		serial:       func([]uint16) bool { return false },
	})
	if err != nil || info.Product != "blink(1) mk2" || info.SerialNumber != "" {
		t.Fatalf("serial failure reused product: %+v, err=%v", info, err)
	}
}

func TestWindowsMetadataAttributeFailureDoesNotConsumePartialOutput(t *testing.T) {
	failure := errors.New("attributes failed")
	info := &DeviceInfo{Path: "fake-path"}
	unexpected := func([]uint16) bool { t.Error("string queried after attribute failure"); return true }
	err := populateWindowsHIDMetadata(info, windowsHIDMetadataQueries{
		attributes:   func() (uint16, uint16, uint16, error) { return 0x27b8, 0x01ed, 2, failure },
		manufacturer: unexpected, product: unexpected, serial: unexpected,
	})
	if err != failure || info.VendorID != 0 || info.ProductID != 0 || info.VersionNumber != 0 {
		t.Fatalf("partial attributes consumed: %+v, err=%v", info, err)
	}
}

func TestWindowsHIDStringRequiresSuccessAndBoundedTerminator(t *testing.T) {
	tests := []struct {
		name  string
		query func([]uint16) bool
		want  string
	}{
		{"unicode", windowsMetadataString("灯💡"), "灯💡"},
		{"empty success", func([]uint16) bool { return true }, ""},
		{"failure with output", func(buffer []uint16) bool { copy(buffer, []uint16{'b', 'a', 'd', 0}); return false }, ""},
		{"unterminated success", func(buffer []uint16) bool {
			for i := range buffer {
				buffer[i] = 'a'
			}
			return true
		}, ""},
		{"final terminator", func(buffer []uint16) bool {
			for i := 0; i < len(buffer)-1; i++ {
				buffer[i] = 'a'
			}
			return true
		}, strings.Repeat("a", windowsHIDStringCodeUnits-1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := readWindowsHIDString(test.query); got != test.want {
				t.Fatalf("string = %q, want %q", got, test.want)
			}
		})
	}
}
