# Windows enumeration and 32-bit ABI validation

The proposed job below was subsequently implemented in nb1's integration workflow.
[Run 42517136819](https://github.com/b1ug/nb1/actions/runs/42517136819), pinned to
GID `7d9978d`, passed all eight jobs, including native Windows amd64 race/vet
and Windows/386 CGO tests under WOW64. This remains software evidence without USB.

`SP_DEVICE_INTERFACE_DETAIL_DATA_A.cbSize` must describe the native fixed
structure, rather than a Go pointer. The ANSI structure is 5 bytes on x86 and
8 bytes on 64-bit Windows. The code uses the C `sizeof` constant and tests that
native layout without opening devices. See the [API contract](https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdigetdeviceinterfacedetaila)
and [Microsoft's packed structure definition](https://github.com/microsoft/win32metadata/blob/main/generation/WinSDK/RecompiledIdlHeaders/um/SetupAPI.h).

Enumeration detail/class/driver string queries now capture native error codes
in the same C call, check both query stages, and borrow zero-initialized Go byte
buffers only for synchronous calls. String copying is bounded by the returned
byte count and allocation. Failed fills, growing sizes, invalid offsets and
missing NUL terminators produce an empty result instead of scanning native
memory. Size probes above 1 MiB are rejected before allocation.

The shared callback tests exercise those failure paths without invoking SetupAPI
or HID. `TestWindowsDetailHeaderMatchesNativeABI` additionally needs Windows CGO;
it is a structure-layout test, not USB evidence. Native asynchronous operation
lifetime/cancellation code is unchanged by this enumeration fix.

## Proposed current Windows/386 CGO job

As checked on 2026-11-09, the official [Go platform table](https://github.com/golang/go/blob/go1.27.0/src/internal/platform/zosarch.go)
still enables CGO for `windows/386`. The official [MSYS2 i686 GCC package](https://packages.msys2.org/packages/mingw-w64-i686-gcc)
is currently 16.2.0-6, built 2026-10-13. [setup-msys2](https://github.com/msys2/setup-msys2/blob/main/README.md#msystem)
still supports MINGW32 and `path-type: inherit`, so this job can use the existing
64-bit Go toolchain and a 32-bit C compiler. It is a proposal pending execution;
no local Windows compiler was installed for this change.

```yaml
windows-386:
  runs-on: windows-2022
  timeout-minutes: 15
  env:
    GOOS: windows
    GOARCH: '386'
    CGO_ENABLED: '1'
    GOWORK: 'off'
    CC: gcc
  steps:
    - uses: actions/checkout@v7
      with:
        persist-credentials: false
    - uses: actions/setup-go@v7
      with:
        go-version: '1.27.x'
        architecture: x64
        cache: false
    - uses: msys2/setup-msys2@v2
      with:
        msystem: MINGW32
        update: true
        install: mingw-w64-i686-gcc
        path-type: inherit
    - name: Build and execute Windows 386 tests
      shell: msys2 {0}
      run: |
        set -euo pipefail
        test "$(gcc -dumpmachine)" = i686-w64-mingw32
        test "$(go env GOOS)" = windows
        test "$(go env GOARCH)" = 386
        test "$(go env CGO_ENABLED)" = 1
        test "$(go env CC)" = gcc
        go version
        go env GOHOSTARCH GOOS GOARCH CGO_ENABLED CC
        gcc -dumpmachine
        go build ./...
        go test -count=1 -timeout=2m ./...
```

Do not add `-race`: Go's [Windows race support](https://go.dev/doc/articles/race_detector#Requirements)
is limited to amd64. This job executes 32-bit processes under [WOW64](https://learn.microsoft.com/en-us/windows/win32/winprog64/running-32-bit-applications)
on the 64-bit runner. It does not prove an older 32-bit OS or physical USB device
works. The Go 1.13 module declaration remains a source compatibility statement,
separate from current Go/Windows runtime requirements.

MINGW32 is deprecated and has no direct MSYS2 replacement; current package
availability is not a perpetual support promise. MSYS2 has phased out some
32-bit packages and removed CLANG32; its [environment history](https://www.msys2.org/docs/environments/#changelog)
and [action deprecation notice](https://github.com/msys2/setup-msys2/blob/main/README.md#msystem)
should be checked when maintaining this job. No workflow was changed or run by
this patch.
