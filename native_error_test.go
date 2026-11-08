package gid_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/b1ug/gid"
)

func TestNativeErrorPreservesCauseAndTraits(t *testing.T) {
	cause := errors.New("original native message")
	native := &gid.NativeError{Platform: "example-native-backend", Code: 0x100000001, Err: cause}
	if native.Error() != cause.Error() || errors.Unwrap(native) != cause {
		t.Fatalf("native error changed cause or message: %q, %v", native.Error(), errors.Unwrap(native))
	}
	wrapped := fmt.Errorf("operation failed: %w", native)
	var details interface {
		NativeCode() uint64
		NativePlatform() string
	}
	if !errors.As(wrapped, &details) || details.NativeCode() != 0x100000001 || details.NativePlatform() != "example-native-backend" {
		t.Fatalf("native details were lost in wrapping: %#v", details)
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("cause identity was lost in wrapping")
	}
}

func TestNativeErrorWithoutCause(t *testing.T) {
	err := &gid.NativeError{Platform: "example-native-backend", Code: 7}
	if err.Error() != "native error" || errors.Unwrap(err) != nil || err.NativeCode() != 7 {
		t.Fatalf("error without a cause = %q, %v, code %d", err.Error(), errors.Unwrap(err), err.NativeCode())
	}
}
