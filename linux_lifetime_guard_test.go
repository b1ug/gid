package gid

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func linuxWaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for " + description)
	}
}

func linuxWaitClosing(t *testing.T, lifetime *linuxLifetime) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !lifetime.isClosing() {
		select {
		case <-deadline:
			t.Fatal("Close did not publish its closing signal")
		case <-ticker.C:
		}
	}
}

func linuxAssertPending(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatal(description)
	default:
	}
}

func TestLinuxLifetimeWaitsForEveryOperationAndFullDisposal(t *testing.T) {
	var lifetime linuxLifetime
	if !lifetime.acquire() || !lifetime.acquire() {
		t.Fatal("could not acquire concurrent native-operation leases")
	}
	disposalStarted := make(chan struct{})
	disposalAllowed := make(chan struct{})
	var allowOnce sync.Once
	defer allowOnce.Do(func() { close(disposalAllowed) })
	var disposalCalls int32
	dispose := func() {
		atomic.AddInt32(&disposalCalls, 1)
		close(disposalStarted)
		<-disposalAllowed
	}
	firstDone, secondDone := make(chan struct{}), make(chan struct{})
	go func() { lifetime.close(dispose); close(firstDone) }()
	linuxWaitClosing(t, &lifetime)
	go func() { lifetime.close(dispose); close(secondDone) }()
	if lifetime.acquire() {
		lifetime.release()
		t.Fatal("new I/O was accepted after Close published closing")
	}
	linuxAssertPending(t, disposalStarted, "native handle disposed with two operations active")
	lifetime.release()
	linuxAssertPending(t, disposalStarted, "native handle disposed with one operation still active")
	lifetime.release()
	linuxWaitSignal(t, disposalStarted, "disposal after all operation leases return")
	linuxAssertPending(t, firstDone, "Close returned before native disposal completed")
	linuxAssertPending(t, secondDone, "concurrent Close returned before native disposal completed")
	allowOnce.Do(func() { close(disposalAllowed) })
	linuxWaitSignal(t, firstDone, "first Close")
	linuxWaitSignal(t, secondDone, "second Close")
	if got := atomic.LoadInt32(&disposalCalls); got != 1 {
		t.Fatalf("native disposal calls = %d, want 1", got)
	}
	lifetime.close(dispose)
	if lifetime.acquire() {
		lifetime.release()
		t.Fatal("operation accepted after completed Close")
	}
}

func TestLinuxLifetimeRetainsHandleDuringErrorCopy(t *testing.T) {
	var lifetime linuxLifetime
	copyStarted, copyAllowed := make(chan struct{}), make(chan struct{})
	operationDone, freed, closeDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var allowOnce sync.Once
	defer allowOnce.Do(func() { close(copyAllowed) })
	go func() {
		if !lifetime.acquire() {
			close(operationDone)
			return
		}
		defer lifetime.release()
		// Model a native call that failed and is now copying its error text.
		// Copying still needs the same native handle, even after the call ended.
		close(copyStarted)
		<-copyAllowed
		close(operationDone)
	}()
	linuxWaitSignal(t, copyStarted, "native error-string copy")
	go func() { lifetime.close(func() { close(freed) }); close(closeDone) }()
	linuxWaitClosing(t, &lifetime)
	linuxAssertPending(t, freed, "handle freed while native error text was still being copied")
	allowOnce.Do(func() { close(copyAllowed) })
	linuxWaitSignal(t, operationDone, "error-string copy completion")
	linuxWaitSignal(t, closeDone, "Close after error-string copy")
	linuxWaitSignal(t, freed, "native handle free")
}

func TestLinuxPollInputStopsAfterClosingWithoutAnotherNativeRead(t *testing.T) {
	var lifetime linuxLifetime
	if !lifetime.acquire() {
		t.Fatal("could not acquire input read lease")
	}
	readEntered, sliceAllowed := make(chan struct{}), make(chan struct{})
	freed, closeDone := make(chan struct{}), make(chan struct{})
	var sliceOnce sync.Once
	defer sliceOnce.Do(func() { close(sliceAllowed) })
	var calls int32
	type result struct {
		n   int
		err error
	}
	readDone := make(chan result, 1)
	go func() {
		defer lifetime.release()
		n, err := linuxPollInput(&lifetime, func(milliseconds int) (int, error) {
			if milliseconds != 100 {
				return 0, errors.New("native read did not receive the 100ms slice")
			}
			if atomic.AddInt32(&calls, 1) == 1 {
				close(readEntered)
				<-sliceAllowed
			}
			return 0, nil // No input report arrived during this native slice.
		})
		readDone <- result{n, err}
	}()
	linuxWaitSignal(t, readEntered, "first native read slice")
	go func() { lifetime.close(func() { close(freed) }); close(closeDone) }()
	linuxWaitClosing(t, &lifetime)
	linuxAssertPending(t, freed, "handle freed before active native read returned")
	sliceOnce.Do(func() { close(sliceAllowed) })
	select {
	case got := <-readDone:
		if got.n != 0 || got.err != errDeviceClosed {
			t.Fatalf("read after Close = %d, %v; want closed error", got.n, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("input read did not return after its current slice")
	}
	linuxWaitSignal(t, closeDone, "Close after input read")
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("native read calls = %d, want 1", got)
	}
}

func TestLinuxLifetimeBlockedReadDoesNotPreventWrite(t *testing.T) {
	var lifetime linuxLifetime
	input := make(chan struct{})
	readEntered, readDone := make(chan struct{}), make(chan struct{})
	writeEntered, writeAllowed, writeDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	closeDone := make(chan struct{})
	var writeOnce sync.Once
	defer writeOnce.Do(func() { close(writeAllowed) })
	readResult := make(chan error, 1)
	go func() {
		defer close(readDone)
		if !lifetime.acquire() {
			readResult <- errors.New("read could not acquire lease")
			return
		}
		defer lifetime.release()
		var enterOnce sync.Once
		n, err := linuxPollInput(&lifetime, func(int) (int, error) {
			enterOnce.Do(func() { close(readEntered) })
			// A real finite native input read may wait for an output command
			// to cause input. Keep this fake under caller-controlled barriers.
			<-input
			return 1, nil
		})
		if err != nil {
			readResult <- err
		} else if n != 1 {
			readResult <- errors.New("input report was lost")
		} else {
			readResult <- nil
		}
	}()
	linuxWaitSignal(t, readEntered, "blocked native input read")
	go func() {
		defer close(writeDone)
		if !lifetime.acquire() {
			return
		}
		defer lifetime.release()
		close(writeEntered)
		<-writeAllowed
		close(input) // This output operation generates the awaited input.
	}()
	linuxWaitSignal(t, writeEntered, "concurrent write while input Read is blocked")
	go func() { lifetime.close(func() {}); close(closeDone) }()
	linuxWaitClosing(t, &lifetime)
	linuxAssertPending(t, closeDone, "Close bypassed active read/write operations")
	writeOnce.Do(func() { close(writeAllowed) })
	linuxWaitSignal(t, writeDone, "write-generated input")
	linuxWaitSignal(t, readDone, "input read response")
	if err := <-readResult; err != nil {
		t.Fatal(err)
	}
	linuxWaitSignal(t, closeDone, "Close after parallel read/write")
}

func TestLinuxPollInputRetainsBlockingAndErrorSemantics(t *testing.T) {
	var lifetime linuxLifetime
	calls := 0
	n, err := linuxPollInput(&lifetime, func(milliseconds int) (int, error) {
		calls++
		if milliseconds != 100 {
			t.Errorf("read timeout = %d, want 100", milliseconds)
		}
		if calls < 3 {
			return 0, nil
		}
		return 7, nil
	})
	if n != 7 || err != nil || calls != 3 {
		t.Fatalf("blocking read = %d, %v, calls=%d; want 7, nil, 3", n, err, calls)
	}
	nativeErr := errors.New("copied native error")
	n, err = linuxPollInput(&lifetime, func(int) (int, error) { return 0, nativeErr })
	if n != 0 || err != nativeErr {
		t.Fatalf("native read failure = %d, %v; want copied native error", n, err)
	}
	lifetime.close(func() {})
	n, err = linuxPollInput(&lifetime, func(int) (int, error) { t.Fatal("native read called after closing"); return 0, nil })
	if n != 0 || err != errDeviceClosed {
		t.Fatalf("already closing read = %d, %v; want closed error", n, err)
	}
}
