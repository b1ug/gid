package gid

import (
	"errors"
	"fmt"
	"io"
)

var (
	errEmptyReportBuffer = errors.New("hid: empty report buffer")
	errEmptyReportData   = errors.New("hid: unnumbered report has no data")
	errReportUnavailable = errors.New("hid: report type is not available")
)

// validateReportBuffer checks the caller-supplied report ID before a native API
// can index the buffer. The zero ID is a placeholder for an unnumbered report;
// stripping that placeholder must leave at least one byte for the native call.
func validateReportBuffer(data []byte) error {
	if len(data) == 0 {
		return errEmptyReportBuffer
	}
	if len(data) == 1 && data[0] == 0 {
		return errEmptyReportData
	}
	return nil
}

// prepareWriteReport pads to the Windows HID collection's report size without
// silently dropping caller-supplied bytes. reportSize includes the report ID.
func prepareWriteReport(data []byte, reportSize int) ([]byte, error) {
	if err := validateReportBuffer(data); err != nil {
		return nil, err
	}
	if reportSize <= 0 {
		return nil, errReportUnavailable
	}
	if len(data) > reportSize {
		return nil, fmt.Errorf("hid: report buffer has %d bytes, maximum is %d", len(data), reportSize)
	}
	buffer := make([]byte, reportSize)
	copy(buffer, data)
	return buffer, nil
}

// copyReadReport reports only bytes actually placed in the caller's buffer.
// Windows may need a larger native buffer for the collection's maximum report.
func copyReadReport(data, report []byte, read int) (int, error) {
	if read < 0 || read > len(report) {
		return 0, errors.New("hid: invalid native report length")
	}
	n := copy(data, report[:read])
	if n < read {
		return n, io.ErrShortBuffer
	}
	return n, nil
}

// Windows HID class may complete GET_INPUT_REPORT with Information=0 even
// though the report buffer is filled. Preserve that buffer and the native zero
// count; do not invent an actual report length from the collection maximum.
// https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidclass/ni-hidclass-ioctl_hid_get_input_report
func copyInputReport(data, report []byte, read int) (int, error) {
	if read == 0 {
		copy(data, report)
		return 0, nil
	}
	return copyReadReport(data, report, read)
}
