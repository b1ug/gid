package gid

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func windowsWaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for " + description)
	}
}

func windowsWaitClosing(t *testing.T, lifetime *windowsLifetime) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for !lifetime.isClosing() {
		select {
		case <-deadline:
			t.Fatal("Close did not publish closing before waiting")
		case <-time.After(time.Millisecond):
		}
	}
}

func windowsAssertPending(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatal(description)
	default:
	}
}

func TestWindowsLifetimeCancelsBeforeWaitingForAllOperations(t *testing.T) {
	var lifetime windowsLifetime
	if !lifetime.acquire() || !lifetime.acquire() {
		t.Fatal("could not acquire concurrent read/write leases")
	}
	cancelStarted, cancelAllowed := make(chan struct{}), make(chan struct{})
	disposeStarted, disposeAllowed := make(chan struct{}), make(chan struct{})
	var allowCancel, allowDispose sync.Once
	defer allowCancel.Do(func() { close(cancelAllowed) })
	defer allowDispose.Do(func() { close(disposeAllowed) })
	var cancels, disposals int32
	cancel := func() { atomic.AddInt32(&cancels, 1); close(cancelStarted); <-cancelAllowed }
	dispose := func() { atomic.AddInt32(&disposals, 1); close(disposeStarted); <-disposeAllowed }
	firstDone, secondDone := make(chan struct{}), make(chan struct{})
	go func() { lifetime.close(cancel, dispose); close(firstDone) }()
	windowsWaitClosing(t, &lifetime)
	windowsWaitSignal(t, cancelStarted, "cancellation while read/write leases are still active")
	go func() { lifetime.close(cancel, dispose); close(secondDone) }()
	if lifetime.acquire() {
		lifetime.release()
		t.Fatal("new operation admitted after closing")
	}
	windowsAssertPending(t, disposeStarted, "handle disposed before cancellation or I/O completion")
	allowCancel.Do(func() { close(cancelAllowed) })
	lifetime.release()
	windowsAssertPending(t, disposeStarted, "handle disposed with one native operation active")
	lifetime.release()
	windowsWaitSignal(t, disposeStarted, "native disposal after every operation")
	windowsAssertPending(t, firstDone, "Close returned before disposal completed")
	windowsAssertPending(t, secondDone, "repeated Close returned before disposal completed")
	allowDispose.Do(func() { close(disposeAllowed) })
	windowsWaitSignal(t, firstDone, "first Close")
	windowsWaitSignal(t, secondDone, "second Close")
	lifetime.close(cancel, dispose)
	if atomic.LoadInt32(&cancels) != 1 || atomic.LoadInt32(&disposals) != 1 {
		t.Fatalf("cancel/dispose count = %d/%d, want 1/1", cancels, disposals)
	}
}

func TestWindowsLifetimeCoversIssueAfterCloseCancellationSnapshot(t *testing.T) {
	var lifetime windowsLifetime
	if !lifetime.acquire() {
		t.Fatal("could not acquire pre-close operation lease")
	}
	snapshotDone, cancelRequested, kernelDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	freed, closeDone, operationDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var kernelOnce sync.Once
	defer kernelOnce.Do(func() { close(kernelDone) })
	var issued, specificCancels int32
	go func() {
		lifetime.close(func() {
			if atomic.LoadInt32(&issued) != 0 {
				t.Error("test failed to hold issuance until after the cancellation snapshot")
			}
			close(snapshotDone)
		}, func() { close(freed) })
		close(closeDone)
	}()
	windowsWaitSignal(t, snapshotDone, "Close's all-I/O cancellation snapshot")
	// Model WriteFile returning IO_PENDING only after the earlier CancelIoEx
	// snapshot. The operation must cancel itself before waiting for completion.
	atomic.StoreInt32(&issued, 1)
	lifetime.afterIssue(func() { atomic.AddInt32(&specificCancels, 1); close(cancelRequested) })
	windowsWaitSignal(t, cancelRequested, "specific cancellation for late-issued I/O")
	windowsAssertPending(t, freed, "cancellation request was mistaken for kernel completion")
	go func() { <-kernelDone; lifetime.release(); close(operationDone) }()
	kernelOnce.Do(func() { close(kernelDone) })
	windowsWaitSignal(t, operationDone, "kernel completion and error-copy lease release")
	windowsWaitSignal(t, closeDone, "Close after late-issued operation")
	if atomic.LoadInt32(&specificCancels) != 1 {
		t.Fatalf("specific cancellations = %d, want 1", specificCancels)
	}
}

func TestWindowsLifetimeRetainsHandleDuringCompletionErrorProcessing(t *testing.T) {
	var lifetime windowsLifetime
	processing, processingAllowed := make(chan struct{}), make(chan struct{})
	freed, done := make(chan struct{}), make(chan struct{})
	var allowed sync.Once
	defer allowed.Do(func() { close(processingAllowed) })
	go func() {
		if !lifetime.acquire() {
			return
		}
		defer lifetime.release()
		close(processing) // Completion failed; native status is being copied.
		<-processingAllowed
	}()
	windowsWaitSignal(t, processing, "completion error processing")
	go func() { lifetime.close(func() {}, func() { close(freed) }); close(done) }()
	windowsWaitClosing(t, &lifetime)
	windowsAssertPending(t, freed, "native handle freed before error processing finished")
	allowed.Do(func() { close(processingAllowed) })
	windowsWaitSignal(t, done, "Close after completion error processing")
	if lifetime.acquire() {
		lifetime.release()
		t.Fatal("operation admitted after completed Close")
	}
}

func TestWindowsLifetimeDoesNotCancelOperationsBeforeClosing(t *testing.T) {
	var lifetime windowsLifetime
	called := false
	lifetime.afterIssue(func() { called = true })
	if called {
		t.Fatal("normal operation was spuriously cancelled")
	}
}
