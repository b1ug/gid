package gid

import (
	"strconv"
	"strings"
)

type darwinDeviceIDs struct{ vendor, product uint16 }

// getPath has historically encoded transport_vendor_product_location. Decode
// its last three fields, so path-only DeviceInfo values still work without
// opening an unfiltered manager. Transport may be empty or contain underscores.
// A malformed path cannot match getPath and must fail before calling IOKit.
func darwinOpenIDs(info *DeviceInfo) (darwinDeviceIDs, error) {
	if info == nil {
		return darwinDeviceIDs{}, errDeviceNotFound
	}
	parts := strings.Split(info.Path, "_")
	if len(parts) < 4 {
		return darwinDeviceIDs{}, errDeviceNotFound
	}
	last := len(parts) - 1
	vendor, vendorOK := fixedHex(parts[last-2], 4, 16)
	product, productOK := fixedHex(parts[last-1], 4, 16)
	_, locationOK := fixedHex(parts[last], 8, 32)
	if !vendorOK || !productOK || !locationOK {
		return darwinDeviceIDs{}, errDeviceNotFound
	}
	ids := darwinDeviceIDs{vendor: uint16(vendor), product: uint16(product)}
	// Both zero fields are also the historical path-only zero value. Every
	// other DeviceInfo must agree with its own path rather than broadening it.
	if (info.VendorID != 0 || info.ProductID != 0) && (info.VendorID != ids.vendor || info.ProductID != ids.product) {
		return darwinDeviceIDs{}, errDeviceNotFound
	}
	return ids, nil
}

func fixedHex(text string, length, bits int) (uint64, bool) {
	if len(text) != length {
		return 0, false
	}
	for _, char := range text {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return 0, false
		}
	}
	value, err := strconv.ParseUint(text, 16, bits)
	return value, err == nil
}
