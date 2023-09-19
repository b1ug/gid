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
