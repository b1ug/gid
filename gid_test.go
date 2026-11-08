package gid_test

import (
	"os"
	"sync"
	"testing"

	"github.com/b1ug/gid"
)

func TestSupported(t *testing.T) {
	if !gid.Supported() {
		t.Error("platform is not supported")
	}
}

func TestThreadedDevices(t *testing.T) {
	var pend sync.WaitGroup
	for i := 0; i < 8; i++ {
		pend.Add(1)

		go func(index int) {
			defer pend.Done()
			for range gid.Devices() {
			}
		}(i)
	}
	pend.Wait()
}

func TestListAll(t *testing.T) {
	for dev := range gid.Devices() {
		t.Logf("%+v", dev)
	}

	devs := gid.ListAllDevices(nil)
	if len(devs) == 0 {
		t.Logf("no devices found")
	} else {
		t.Logf("found %d devices", len(devs))
	}
}

func TestOpenSelectedDevice(t *testing.T) {
	path := os.Getenv("GID_TEST_DEVICE_PATH")
	if path == "" {
		t.Skip("set GID_TEST_DEVICE_PATH to opt in to opening a specific HID device")
	}
	dev, err := gid.ByPath(path)
	if err != nil {
		t.Fatalf("find selected device: %v", err)
	}
	d, err := dev.Open()
	if err != nil {
		t.Fatalf("open selected device: %v", err)
	}
	if d == nil {
		t.Fatal("Open returned a nil device without an error")
	}
	defer d.Close()
}
