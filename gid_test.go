package gid_test

import (
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

func TestOpenFirst(t *testing.T) {
	dev := gid.ListFirstDevice(nil)
	if dev == nil {
		t.Logf("no devices found")
	} else {
		t.Logf("found 1st device: %+v", dev)
		d, err := dev.Open()
		if err != nil {
			t.Logf("can't open 1st device: %v", err)
		} else {
			t.Logf("opened 1st device: %+v", d)
			d.Close()
		}
	}
}
