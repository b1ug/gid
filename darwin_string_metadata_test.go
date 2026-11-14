//go:build darwin && cgo && !ios
// +build darwin,cgo,!ios

package gid

import "testing"

func TestDarwinStringMetadataTypeBoundary(t *testing.T) {
	// The same untyped-property conversion is used by getStringProp. These
	// existing CF-only fixtures contain nil, CFString and CFNumber values;
	// no HID manager, enumeration or device handle is involved.
	for _, test := range []struct {
		name string
		kind int
		want string
	}{
		{"missing", 0, ""},
		{"string", 1, "not a numeric HID ID"},
		{"number_zero", 2, ""},
		{"number_nonzero", 3, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			properties := darwinPropertyFixture(test.kind, 0, 0x27b8, 0)
			if properties.ref == 0 {
				t.Fatal("CF fixture allocation failed")
			}
			defer properties.close()
			if got := properties.fixtureVendorString(); got != test.want {
				t.Fatalf("metadata string = %q, want %q", got, test.want)
			}
		})
	}
}
