package gid

import (
	"reflect"
	"testing"
)

func TestDarwinOpenIDPolicyPreservesPathsWithoutBroadening(t *testing.T) {
	for _, test := range []struct {
		name string
		info *DeviceInfo
		want darwinDeviceIDs
		ok   bool
	}{
		{"real blink", &DeviceInfo{Path: "USB_27b8_01ed_00100000", VendorID: 0x27b8, ProductID: 0x01ed}, darwinDeviceIDs{0x27b8, 0x01ed}, true},
		{"path only", &DeviceInfo{Path: "USB_27b8_01ed_00100000"}, darwinDeviceIDs{0x27b8, 0x01ed}, true},
		{"transport underscore", &DeviceInfo{Path: "custom_transport_name_27b8_01ed_00100000"}, darwinDeviceIDs{0x27b8, 0x01ed}, true},
		{"missing transport property", &DeviceInfo{Path: "_27b8_01ed_00100000"}, darwinDeviceIDs{0x27b8, 0x01ed}, true},
		{"zero vendor is exact", &DeviceInfo{Path: "USB_0000_01ed_00000000", ProductID: 0x01ed}, darwinDeviceIDs{0, 0x01ed}, true},
		{"zero product is exact", &DeviceInfo{Path: "USB_27b8_0000_00000000", VendorID: 0x27b8}, darwinDeviceIDs{0x27b8, 0}, true},
		{"both zero are exact", &DeviceInfo{Path: "USB_0000_0000_00000000"}, darwinDeviceIDs{0, 0}, true},
		{"max IDs", &DeviceInfo{Path: "USB_ffff_ffff_ffffffff"}, darwinDeviceIDs{0xffff, 0xffff}, true},
		{"nil info", nil, darwinDeviceIDs{}, false},
		{"empty", &DeviceInfo{}, darwinDeviceIDs{}, false},
		{"unstructured", &DeviceInfo{Path: "never-a-real-hid-path"}, darwinDeviceIDs{}, false},
		{"no transport field", &DeviceInfo{Path: "27b8_01ed_00000000"}, darwinDeviceIDs{}, false},
		{"short vendor", &DeviceInfo{Path: "USB_27b_01ed_00000000"}, darwinDeviceIDs{}, false},
		{"short product", &DeviceInfo{Path: "USB_27b8_1ed_00000000"}, darwinDeviceIDs{}, false},
		{"short location", &DeviceInfo{Path: "USB_27b8_01ed_0000000"}, darwinDeviceIDs{}, false},
		{"signed hex rejected", &DeviceInfo{Path: "USB_+7b8_01ed_00000000"}, darwinDeviceIDs{}, false},
		{"nonhex location", &DeviceInfo{Path: "USB_27b8_01ed_0000000z"}, darwinDeviceIDs{}, false},
		{"vendor mismatch", &DeviceInfo{Path: "USB_27b8_01ed_00000000", VendorID: 0x1234, ProductID: 0x01ed}, darwinDeviceIDs{}, false},
		{"product mismatch", &DeviceInfo{Path: "USB_27b8_01ed_00000000", VendorID: 0x27b8, ProductID: 0x1234}, darwinDeviceIDs{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := darwinOpenIDs(test.info)
			if (err == nil) != test.ok || test.ok && got != test.want {
				t.Fatalf("match=%+v err=%v, want=%+v ok=%t", got, err, test.want, test.ok)
			}
			if !test.ok && err != errDeviceNotFound {
				t.Fatalf("invalid path compatibility error=%v", err)
			}
		})
	}
}

func TestFindDevicesFilterTreatsZeroAsAnExactID(t *testing.T) {
	for _, wanted := range []darwinDeviceIDs{{0, 0}, {0, 0x01ed}, {0x27b8, 0}, {0x27b8, 0x01ed}} {
		input := make(chan *DeviceInfo, 4)
		for _, info := range []*DeviceInfo{
			{VendorID: 0, ProductID: 0, Path: "both-zero"},
			{VendorID: 0, ProductID: 0x01ed, Path: "vendor-zero"},
			{VendorID: 0x27b8, ProductID: 0, Path: "product-zero"},
			{VendorID: 0x27b8, ProductID: 0x01ed, Path: "blink"},
		} {
			input <- info
		}
		close(input)
		var got []darwinDeviceIDs
		for info := range filterDeviceIDs(input, wanted.vendor, wanted.product) {
			got = append(got, darwinDeviceIDs{info.VendorID, info.ProductID})
		}
		if !reflect.DeepEqual(got, []darwinDeviceIDs{wanted}) {
			t.Fatalf("zero became a wildcard: want=%+v got=%+v", wanted, got)
		}
	}
}
