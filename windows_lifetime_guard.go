package gid

import (
	"sync"
	"sync/atomic"
)

// windowsLifetime keeps the native handle alive for complete operations while
// still allowing independent reads and writes to overlap. Cancellation requests
// must not release the handle or operation buffers before completion is known.
// The zero value is ready to use and must not be copied after first use.
type windowsLifetime struct {
	gate    sync.RWMutex
	closing uint32
	once    sync.Once
}

func (l *windowsLifetime) acquire() bool {
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

func (l *windowsLifetime) release()        { l.gate.RUnlock() }
func (l *windowsLifetime) isClosing() bool { return atomic.LoadUint32(&l.closing) != 0 }

func (l *windowsLifetime) close(cancel, dispose func()) {
	atomic.StoreUint32(&l.closing, 1)
	l.once.Do(func() {
		// Request cancellation while the handle is pinned, before taking the
		// exclusive lock that waits for pending overlapped operations to finish.
		func() {
			l.gate.RLock()
			defer l.gate.RUnlock()
			cancel()
		}()
		l.gate.Lock()
		defer l.gate.Unlock()
		dispose()
	})
}

func (l *windowsLifetime) afterIssue(cancel func()) {
	// An operation may have acquired its lease before closing but issued I/O
	// after Close's CancelIoEx(handle,nil) snapshot. Cover that race by canceling
	// this operation after issuance, while its handle/buffer/OVERLAPPED are pinned.
	if l.isClosing() {
		cancel()
	}
}
