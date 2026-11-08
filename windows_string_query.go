package gid

import "bytes"

const (
	windowsStringSuccess            uint32 = 0
	windowsStringInsufficientBuffer uint32 = 122
	// Enumeration strings should not permit an unbounded allocation based on
	// a failed or inconsistent native size probe. This includes any header.
	maxWindowsStringBytes = 1 << 20
)

// windowsStringQuery returns the offset of its ANSI string, the required or
// populated byte count (including a terminator), and the native error code.
// Each callback must capture GetLastError in the same C call that failed.
type windowsStringQuery func(buffer []byte) (offset int, size uint32, code uint32)

// getCString performs a bounded two-call query. Its zero-initialized byte buffer
// may be borrowed by a synchronous native call, never retained afterwards.
// Failed fills and unterminated results cannot cause an unbounded C.GoString
// scan or expose bytes from an uninitialized native allocation.
func getCString(query windowsStringQuery) string {
	_, required, code := query(nil)
	if code != windowsStringSuccess && code != windowsStringInsufficientBuffer {
		return ""
	}
	if required == 0 || required > maxWindowsStringBytes {
		return ""
	}
	buffer := make([]byte, int(required))
	offset, size, code := query(buffer)
	if code != windowsStringSuccess || size == 0 || size > uint32(len(buffer)) || offset < 0 || offset >= int(size) {
		return ""
	}
	stringBytes := buffer[offset:int(size)]
	end := bytes.IndexByte(stringBytes, 0)
	if end < 0 {
		return ""
	}
	return string(stringBytes[:end])
}
