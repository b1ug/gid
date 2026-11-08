package gid

import (
	"runtime"
	"strings"
	"testing"
)

func TestDarwinStringRoundTrip(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"empty", ""},
		{"ASCII", "blink(1) mk2"},
		{"accent", "caf\u00e9"},
		{"Chinese", "\u8bbe\u5907\u540d\u79f0"},
		{"emoji", "USB \U0001f4a1 \U0001f600"},
		{"embedded NUL", "before\x00after"},
		{"long", strings.Repeat("long \u8bbe\u5907 \U0001f4a1 ", 1024)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := cfstring(tc.value)
			if ref == 0 {
				t.Fatal("CFStringCreateWithBytes failed")
			}
			got := gostring(ref)
			releaseCFString(ref)
			runtime.GC()
			if got != tc.value {
				t.Errorf("round trip = %q, want %q", got, tc.value)
			}
		})
	}
}

func TestDarwinNilString(t *testing.T) {
	if got := gostring(0); got != "" {
		t.Errorf("nil CFString = %q, want empty string", got)
	}
	releaseCFString(0)
}

func TestDarwinOpenMissingDevice(t *testing.T) {
	// Enumerating the host's devices is safe here: this path cannot match the
	// transport/vendor/product/location path produced by getPath.
	di := DeviceInfo{Path: "gid-test-device-that-does-not-exist"}
	dev, err := di.Open()
	if dev != nil {
		t.Fatalf("Open missing device returned non-nil interface: %T", dev)
	}
	if err != errDeviceNotFound {
		t.Errorf("Open missing device error = %v, want %v", err, errDeviceNotFound)
	}
}
