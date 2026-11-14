# gid - Gopher Interface Devices

[![Go Reference](https://pkg.go.dev/badge/github.com/b1ug/gid.svg)](https://pkg.go.dev/github.com/b1ug/gid)
[![Go Report Card](https://goreportcard.com/badge/github.com/b1ug/gid)](https://goreportcard.com/report/github.com/b1ug/gid)
[![GitHub Actions](https://github.com/b1ug/gid/actions/workflows/build.yml/badge.svg)](https://github.com/b1ug/gid/actions/workflows/build.yml)
[![CircleCI](https://dl.circleci.com/status-badge/img/gh/b1ug/gid/tree/master.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/b1ug/gid/tree/master)

> Yet another Go package tailored for interacting with USB Human Interface Devices (HID).

This package facilitates cross-platform communications with HID devices on macOS, Linux, and Windows. Behind the scenes, it employs [boombuler/hid](https://github.com/boombuler/hid) as its foundation and broadens its functionalities by reading feature reports from USB HID devices, and its Linux implementation is a derivative of [dolmen-go/hid](https://github.com/dolmen-go/hid), which wraps [`hidapi`](https://github.com/signal11/hidapi).

Divergent from comprehensive USB HID packages, `gid` chiefly focuses on offering support limited to HID devices with feature reports across major platforms. Our primary spotlight is the [ThingM blink(1)](https://blink1.thingm.com/) devices.

## Compatible Operating Systems

The native backends target the following systems:

* macOS: Through its native IOKit framework
* Linux: Leverages libusb 1.0+
* Windows: Utilizes native Windows HID library

The original project recorded tests on these systems. These historical results
do not establish compatibility of the current candidate with those OS versions:

* macOS Ventura 13.5.2 (on Apple M1 Chip)
* Windows 10 x64 - Build 19045
* Windows 11 x64 - Build 22621
* Raspberry Pi OS 64-bit Bullseye (on Raspberry Pi 3 Model B)

The original hardware test target was:

* ThingM blink(1) mk2

## Limitations

This package is designed to support minimum USB HID functionality needed for ThingM blink(1) on major platforms only. Therefore, not all HID features have been implemented.

## Continuous integration and hardware tests

The repository's native CI configuration builds the CGO backend and runs tests (including the Go race detector)
on Ubuntu 24.04 amd64/arm64, macOS 15 arm64/Intel, and Windows Server 2022 amd64,
with Go 1.26.x and 1.27.x. The runner labels are explicit; see GitHub's
[current standard runner list](https://docs.github.com/en/actions/reference/runners/github-hosted-runners#standard-github-hosted-runners-for-public-repositories).
Go 1.13.15 is checked separately on Ubuntu 22.04 to retain the module's source
compatibility floor. This does not establish a minimum supported OS version or
verify old systems, Raspberry Pi OS, or device I/O on hosted runners.

The current development candidate is on `validation/runtime-gid`; it is separate
from the frozen [PR #2](https://github.com/b1ug/gid/pull/2) at `f520a7e`.
CI results apply to their exact checked-out commits. The [runtime integration run](https://github.com/b1ug/nb1/actions/runs/42517136819)
passed all eight jobs with GID `7d9978d`, including Windows amd64 and 386/WOW64
software tests. See the [Chinese candidate validation record](docs/candidate-validation.zh-CN.md)
for the commits, failed attempt, local checks and remaining hardware work.

By default, tests do not enumerate or open host HID devices. Real enumeration
requires `GID_TEST_ENUMERATE=1`. To opt in to an open/close smoke test, select an
exact device path from an explicitly authorized `gid.Devices()` enumeration:

```sh
GID_TEST_DEVICE_PATH='your-platform-specific-device-path' go test -run '^TestOpenSelectedDevice$' -v .
```

The selected-device test fails if that device cannot be found or opened. It does
not write reports. Report I/O, firmware compatibility, unplug/replug behavior,
and concurrent access still require separate tests with the intended hardware.
`CGO_ENABLED=0` selects an unsupported-platform stub; a successful build in that
mode is not evidence that the native HID backend works.

## Report buffers and device lifetime

Output and feature reports start with a report ID; use zero for an unnumbered
report and include at least one payload byte after that zero. Empty buffers and
a lone zero ID return an error. This also replaces Linux's former empty-buffer
no-op. Linux input `Read` remains a stream read and accepts any nonempty buffer.

Windows pads writes to the collection's reported size and rejects oversized
data instead of silently truncating it. Counted reads return only the number of
bytes placed in the caller's buffer, with `io.ErrShortBuffer` if necessary.
Windows input-report IOCTLs may return a zero byte count despite filling the
buffer; that native count and the filled data are preserved. Input `Read` has
platform-specific behavior and is not a portable event-stream contract.

On macOS, synchronous I/O and `Close` hold the same lock for the complete native
operation. Repeated `Close` is safe, and closed-handle operations return errors.
The synchronous backend does not register asynchronous removal callbacks;
disconnects surface as I/O errors, and callers must still call `Close` and open
a new device after re-enumeration. `Close` waits for pending native I/O and has
no guaranteed deadline. Physical unplug/replug still requires hardware testing.

In the current Linux candidate, every implemented I/O operation pins the native
handle through the call and error-string copy. `Close` rejects new operations,
waits for active leases and disposes once. Input `Read` uses internal 100 ms
native wait slices to observe closing while retaining its blocking public
behavior. Reads and writes can overlap; a write that causes input is not blocked
behind the waiting read. The slice is not a close deadline: native calls, the
vendored clock behavior and the read-thread join can still delay completion.

In the current Windows candidate, `Close` publishes closing, requests
`CancelIoEx`, waits for active operations and closes the handle once. Each
overlapped operation retains its own C-allocated payload, `OVERLAPPED` and event
until native completion is collected. A late-issued operation also requests its
own cancellation if closing has begun. Requesting cancellation does not release
resources early. Synchronous `HidD_SetFeature` and driver behavior can still
delay `Close`. Native Windows software tests cover pending named-pipe cancellation;
they do not validate physical HID or guarantee a close deadline.
The enumeration candidate also bounds string copying and uses the native C
detail-structure size; HID metadata failures leave optional strings empty rather
than reusing another field. See the [Windows validation notes](docs/windows-enumeration-validation.md)
and [metadata regression record](docs/windows-hid-metadata-validation.zh-CN.md).

These lifetime guards do not serialize a multi-call application protocol. A
request-write/response-read transaction still needs an application-level owner
or lock. No operation provides a portable context or timeout contract, and
disconnect recovery still requires closing, re-enumerating and opening a new
handle. Physical Linux/Windows USB and blink(1) mk3 behavior remain unverified.

Darwin native errors now preserve their existing text while exposing the raw
IOKit code through `NativePlatform()` / `NativeCode()`. Other platforms' errors
have not been unified into that diagnostic contract. See [native error details](docs/native-errors.md).

The Darwin metadata candidate no longer opens a whole HID manager while
enumerating. `FindDevices` prefilters nonzero VID/PID values in IOKit and retains
exact Go filtering for zero metadata IDs, including legacy missing/non-number
properties normalized to zero. `Devices` retains all-device metadata discovery.
`DeviceInfo.Open` uses the same candidate policy and opens only the exact
selected path. Real host enumeration
tests require `GID_TEST_ENUMERATE=1`, and actual device opening still requires
`GID_TEST_DEVICE_PATH`. Skipped hardware tests are not hardware validation.
See the [Darwin matching and test boundaries](docs/darwin-device-matching.zh-CN.md).
Optional Darwin string metadata is type-checked before calling CoreFoundation
string APIs; missing or non-string values become empty strings. See the
[Darwin string metadata boundary](docs/darwin-string-metadata.md) and its
software-only crash regression test.

On Linux, `FindDevices(vendor, product)` passes IDs to HIDAPI before its
string-query device opens, then retains exact Go equality: zero IDs in the
public API are literal values, even though native HIDAPI uses them as wildcard
candidates. `Devices()` keeps full discovery. The embedded libusb can still
attempt earlier USB-node read/write access on old or unavailable sysfs paths;
this is not a promise that discovery never opens unrelated devices. Windows
discovery uses zero-access metadata handles, not generic read/write handles.
See the [Linux discovery scope and software validation](docs/linux-discovery-permissions.zh-CN.md).

## Credits and License

[![License: MIT](https://img.shields.io/:license-MIT-blue.svg)](http://opensource.org/licenses/MIT)

The `gid` package owes its genesis to the following repositories:

* [boombuler/hid](https://github.com/boombuler/hid)
* [dolmen-go/hid](https://github.com/dolmen-go/hid)

The included dependencies carry the following licenses:

* `hidapi`: [3-clause BSD](https://github.com/signal11/hidapi/blob/master/LICENSE-bsd.txt)
* `libusb`: [GNU LGPL 2.1](https://github.com/libusb/libusb/blob/master/COPYING)
* `gowchar`: [3-clause BSD](https://github.com/orofarne/gowchar/blob/master/LICENSE)

Being open-source, the `gid` package is licensed under the widely-accepted MIT license.
