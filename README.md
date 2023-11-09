# gid - Gopher Interface Devices

[![Go Reference](https://pkg.go.dev/badge/github.com/b1ug/gid.svg)](https://pkg.go.dev/github.com/b1ug/gid)
[![Go Report Card](https://goreportcard.com/badge/github.com/b1ug/gid)](https://goreportcard.com/report/github.com/b1ug/gid)
[![GitHub Actions](https://github.com/b1ug/gid/actions/workflows/build.yml/badge.svg)](https://github.com/b1ug/gid/actions/workflows/build.yml)
[![CircleCI](https://dl.circleci.com/status-badge/img/gh/b1ug/gid/tree/master.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/b1ug/gid/tree/master)

> Yet another Go package tailored for interacting with USB Human Interface Devices (HID).

This package facilitates cross-platform communications with HID devices on macOS, Linux, and Windows. Behind the scenes, it employs [boombuler/hid](https://github.com/boombuler/hid) as its foundation and broadens its functionalities by reading feature reports from USB HID devices, and its Linux implementation is a derivative of [dolmen-go/hid](https://github.com/dolmen-go/hid), which wraps [`hidapi`](https://github.com/signal11/hidapi).

Divergent from comprehensive USB HID packages, `gid` chiefly focuses on offering support limited to HID devices with feature reports across major platforms. Our primary spotlight is the [ThingM blink(1)](https://blink1.thingm.com/) devices.

## Compatible Operating Systems

`gid` is designed to work seamlessly with the following systems:

* macOS: Through its native IOKit framework
* Linux: Leverages libusb 1.0+
* Windows: Utilizes native Windows HID library

We've tested the package on different platforms to ensure its compatibility:

* macOS Venture 13.5.2 (on Apple M1 Chip)
* Windows 10 x64 - Build 19045
* Windows 11 x64 - Build 22621
* Raspberry Pi OS 64-bit Bullseye (on Raspberry Pi 3 Model B)

The package has been verified to work with the following devices:

* ThingM blink(1) mk2

## Limitations

This package is designed to support minimum USB HID functionality needed for ThingM blink(1) on major platforms only. Therefore, not all HID features have been implemented.

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
