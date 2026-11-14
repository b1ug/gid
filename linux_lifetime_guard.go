package gid

import (
	"sync"
	"sync/atomic"
)

// linuxLifetime pins a native handle for complete operations without serializing
// readers and writers against each other. A blocked input read must not prevent
// a write that causes the device to send the input being awaited.
//
// Once closing is published, no new operation may enter native code. Close
// waits for every acquired lease, including native error-string copying, before
// disposing of the handle exactly once. Disposal itself may still block.
// The zero value is ready to use and must not be copied after first use.
type linuxLifetime struct {
	gate    sync.RWMutex
	closing uint32
	once    sync.Once
}

func (l *linuxLifetime) acquire() bool {
	if l.isClosing() {
		return false
	}
	l.gate.RLock()
	if l.isClosing() {
		l.gate.RUnlock()
		return false
	}
	return true
}

func (l *linuxLifetime) release() { l.gate.RUnlock() }

func (l *linuxLifetime) isClosing() bool { return atomic.LoadUint32(&l.closing) != 0 }

func (l *linuxLifetime) close(dispose func()) {
	// Signal before waiting for active operations. In particular, an input read
	// will observe this after its current finite native wait and return its lease.
	atomic.StoreUint32(&l.closing, 1)
	l.once.Do(func() {
		l.gate.Lock()
		defer l.gate.Unlock()
		dispose()
	})
}

const linuxReadPollMilliseconds = 100

// linuxPollInput keeps the public blocking-read contract: native timeout zeros
// stay internal. The caller retains its lifetime lease for this complete loop,
// including error conversion in read. Concurrent operations may hold other
// read leases; Close can publish its signal without waiting for this lease.
//
// A slice is not a hard deadline: the vendored C code uses CLOCK_REALTIME, and
// native transfer cleanup/joins and other feature operations may still block.
func linuxPollInput(lifetime *linuxLifetime, read func(int) (int, error)) (int, error) {
	for {
		if lifetime.isClosing() {
			return 0, errDeviceClosed
		}
		n, err := read(linuxReadPollMilliseconds)
		if n != 0 || err != nil {
			return n, err
		}
	}
}
