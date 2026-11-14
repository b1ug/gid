# Darwin string metadata boundary

`IOHIDDeviceGetProperty` returns a CoreFoundation object. GID now verifies that
the object is a `CFString` before calling `CFStringGetLength` or
`CFStringGetBytes`. A missing or non-string optional property becomes an empty
Go string; valid string metadata, including Unicode and embedded NUL, retains
the existing conversion behavior.

Without this check, passing a valid `CFNumber` through the string-property path
raises an Objective-C exception and aborts the process. A software regression
test reproduces that failure using CoreFoundation dictionaries and verifies the
fixed behavior for missing, string, zero-number and nonzero-number properties.
The test never creates a HID manager, enumerates devices or opens hardware.

This is a metadata type-safety fix. It does not establish that a connected
device has supplied malformed metadata, explain an `IOHIDDeviceSetReport`
failure, change report buffers or retry policy, or provide an I/O/Close deadline.
Physical disconnects and other operating systems still need their own evidence.
