//go:build windows && cgo
// +build windows,cgo

package gid

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var windowsTestKernel32 = syscall.NewLazyDLL("kernel32.dll")
var windowsTestCreateNamedPipe = windowsTestKernel32.NewProc("CreateNamedPipeW")
var windowsTestConnectNamedPipe = windowsTestKernel32.NewProc("ConnectNamedPipe")
var windowsTestPeekNamedPipe = windowsTestKernel32.NewProc("PeekNamedPipe")

func windowsWritePipe(t *testing.T) (syscall.Handle, syscall.Handle) {
	t.Helper()
	path, err := syscall.UTF16PtrFromString(fmt.Sprintf(`\\.\pipe\gid-lifetime-%d-%d`, os.Getpid(), time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	// The client intentionally does not read. A report larger than this pipe's
	// output quota exercises real pending WriteFile/GetOverlappedResult, not a
	// HID IOCTL emulation or an attached USB device.
	server, _, callErr := windowsTestCreateNamedPipe.Call(uintptr(unsafe.Pointer(path)),
		uintptr(0x00000002|syscall.FILE_FLAG_OVERLAPPED), // PIPE_ACCESS_OUTBOUND
		0, 1, 1024, 1024, 0, 0) // byte mode, wait mode
	runtime.KeepAlive(path)
	handle := syscall.Handle(server)
	if !validWindowsHandle(handle) {
		t.Fatalf("CreateNamedPipeW: %v", callErr)
	}
	client, err := syscall.CreateFile(path, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		syscall.CloseHandle(handle)
		t.Fatalf("open test pipe client: %v", err)
	}
	// A successful client open may precede ConnectNamedPipe. The connection is
	// already established, so this acknowledges it without an asynchronous
	// Connect operation retaining any Go-owned OVERLAPPED memory.
	connected, _, connectErr := windowsTestConnectNamedPipe.Call(uintptr(handle), 0)
	if connected == 0 && !errors.Is(connectErr, syscall.Errno(535)) { // ERROR_PIPE_CONNECTED
		syscall.CloseHandle(client)
		syscall.CloseHandle(handle)
		t.Fatalf("ConnectNamedPipe on connected pipe: %v", connectErr)
	}
	return handle, client
}

func TestWindowsCloseCancelsPendingNativePipeWrite(t *testing.T) {
	server, client := windowsWritePipe(t)
	dev := &winDevice{handle: server, info: &DeviceInfo{OutputReportLength: 65535}}
	defer func() {
		// If an assertion fails, closing the peer also lets the kernel complete
		// pending pipe writes before the final device cleanup waits for them.
		syscall.CloseHandle(client)
		dev.Close()
	}()
	data := make([]byte, 65535)
	data[0], data[1] = 1, 42
	written := make(chan error, 1)
	go func() { written <- dev.Write(data) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-written:
			t.Fatalf("test write did not remain pending under pipe backpressure: %v", err)
		default:
		}
		var available uint32
		ok, _, err := windowsTestPeekNamedPipe.Call(uintptr(client), 0, 0, 0, uintptr(unsafe.Pointer(&available)), 0)
		if ok == 0 {
			t.Fatalf("PeekNamedPipe: %v", err)
		}
		if available > 0 {
			break
		} // The actual native write has been issued.
		if time.Now().After(deadline) {
			t.Fatal("native pipe write was not issued")
		}
		time.Sleep(time.Millisecond)
	}
	runtime.GC() // In-flight buffers/OVERLAPPED must not depend on Go heap pins.
	closed := make(chan struct{})
	go func() { dev.Close(); close(closed) }()
	select {
	case err := <-written:
		if !errors.Is(err, syscall.ERROR_OPERATION_ABORTED) {
			t.Fatalf("pending native write returned %v, want ERROR_OPERATION_ABORTED", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CancelIoEx did not complete the pending native pipe write")
	}
	windowsWaitSignal(t, closed, "Close after canceled native pipe write")
	if err := dev.Write([]byte{1, 42}); err != errDeviceClosed {
		t.Fatalf("write after Close = %v, want closed error", err)
	}
	dev.Close()
}
