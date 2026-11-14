package gid

import "unicode/utf16"

const windowsHIDStringCodeUnits = 256

type windowsHIDMetadataQueries struct {
	attributes   func() (vendor, product, version uint16, err error)
	manufacturer func([]uint16) bool
	product      func([]uint16) bool
	serial       func([]uint16) bool
}

// populateWindowsHIDMetadata does not consume attributes from a failed query.
// Optional strings can be absent independently; one failure must never copy a
// different field's value into the device identity.
func populateWindowsHIDMetadata(info *DeviceInfo, queries windowsHIDMetadataQueries) error {
	vendor, product, version, err := queries.attributes()
	if err != nil {
		return err
	}
	info.VendorID, info.ProductID, info.VersionNumber = vendor, product, version
	info.Manufacturer = readWindowsHIDString(queries.manufacturer)
	info.Product = readWindowsHIDString(queries.product)
	info.SerialNumber = readWindowsHIDString(queries.serial)
	return nil
}

// readWindowsHIDString supplies an independent zero-initialized UTF-16 buffer
// for each synchronous HidD query. Only TRUE guarantees a complete string; even
// then, require a terminator inside the allocated buffer before decoding it.
func readWindowsHIDString(query func([]uint16) bool) string {
	buffer := make([]uint16, windowsHIDStringCodeUnits)
	if !query(buffer) {
		return ""
	}
	for end, codeUnit := range buffer {
		if codeUnit == 0 {
			return string(utf16.Decode(buffer[:end]))
		}
	}
	return ""
}
