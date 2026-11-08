//go:build darwin && cgo
// +build darwin,cgo

package gid

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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

func TestDarwinClosedDeviceIO(t *testing.T) {
	dev := &osxDevice{}
	dev.Close()
	dev.Close()

	for _, tc := range []struct {
		name string
		call func([]byte) error
	}{
		{"Write", dev.Write},
		{"WriteFeature", dev.WriteFeature},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call([]byte{1, 0}); err != errDeviceClosed {
				t.Fatalf("closed device error = %v, want %v", err, errDeviceClosed)
			}
		})
	}
	for _, tc := range []struct {
		name string
		call func([]byte) (int, error)
	}{
		{"Read", dev.Read},
		{"ReadFeature", dev.ReadFeature},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, err := tc.call([]byte{1, 0})
			if n != 0 || err != errDeviceClosed {
				t.Fatalf("closed device result = (%d, %v), want (0, %v)", n, err, errDeviceClosed)
			}
		})
	}
}

func TestDarwinConcurrentCloseAndClosedIO(t *testing.T) {
	// No native handle is used: this test is safe on hosts with arbitrary HID
	// devices. A retained manager still needs to be released exactly once.
	var cleanupCalls int32
	dev := &osxDevice{closeDM: func() { atomic.AddInt32(&cleanupCalls, 1) }}
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 32; j++ {
				dev.Close()
				if err := dev.WriteFeature([]byte{1, 0}); err != errDeviceClosed {
					t.Errorf("WriteFeature after close = %v", err)
				}
				if n, err := dev.ReadFeature([]byte{1, 0}); n != 0 || err != errDeviceClosed {
					t.Errorf("ReadFeature after close = (%d, %v)", n, err)
				}
			}
		}()
	}
	workers.Wait()
	if got := atomic.LoadInt32(&cleanupCalls); got != 1 {
		t.Fatalf("manager cleanup calls = %d, want 1", got)
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
