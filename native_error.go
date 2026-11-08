package gid

// NativeError retains a backend's native error code alongside its original
// cause. Its text is the cause's text, without a new platform or code prefix.
// Code interpretation is platform-specific and does not imply retryability.
type NativeError struct {
	Platform string
	Code     uint64
	Err      error
}

func (e *NativeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "native error"
}

// Unwrap preserves errors.Is/errors.As traversal to the original cause.
func (e *NativeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NativeCode returns the uninterpreted native value. Darwin IOReturn values
// are preserved as unsigned 32-bit bit patterns within this uint64 field.
func (e *NativeError) NativeCode() uint64 {
	if e == nil {
		return 0
	}
	return e.Code
}

// NativePlatform identifies the backend's code namespace. Darwin uses "darwin".
// Applications may discover this method and NativeCode through errors.As to an
// interface, without depending on this concrete error type.
func (e *NativeError) NativePlatform() string {
	if e == nil {
		return ""
	}
	return e.Platform
}
