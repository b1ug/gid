package gid

import (
	"bytes"
	"io"
	"testing"
)

func TestReportBufferBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"nil", nil, errEmptyReportBuffer},
		{"empty", []byte{}, errEmptyReportBuffer},
		{"zero ID only", []byte{0}, errEmptyReportData},
		{"numbered ID only", []byte{1}, nil},
		{"unnumbered payload", []byte{0, 42}, nil},
		{"numbered payload", []byte{1, 42}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateReportBuffer(tc.data); got != tc.want {
				t.Fatalf("validation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPrepareWriteReport(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		size int
		want []byte
	}{
		{"exact numbered", []byte{1, 42}, 2, []byte{1, 42}},
		{"exact unnumbered", []byte{0, 42}, 2, []byte{0, 42}},
		{"pad", []byte{1, 42}, 4, []byte{1, 42, 0, 0}},
		{"numbered ID only", []byte{2}, 2, []byte{2, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]byte(nil), tc.data...)
			got, err := prepareWriteReport(tc.data, tc.size)
			if err != nil || !bytes.Equal(got, tc.want) {
				t.Fatalf("report = %v, %v; want %v, nil", got, err, tc.want)
			}
			got[0]++
			if !bytes.Equal(tc.data, original) {
				t.Fatal("report preparation modified caller data")
			}
		})
	}
}

func TestPrepareWriteReportRejectsInvalidSize(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		size int
	}{
		{"empty", nil, 9},
		{"zero ID only", []byte{0}, 9},
		{"missing capability", []byte{1, 42}, 0},
		{"negative capability", []byte{1, 42}, -1},
		{"oversized", []byte{1, 42, 43}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := prepareWriteReport(tc.data, tc.size)
			if err == nil || got != nil {
				t.Fatalf("report = %v, %v; want nil, error", got, err)
			}
		})
	}
}

func TestCopyReadReport(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
		read int
		want []byte
		err  error
	}{
		{"exact", 3, 3, []byte{1, 42, 43}, nil},
		{"short native report", 5, 2, []byte{1, 42, 0, 0, 0}, nil},
		{"short caller buffer", 2, 3, []byte{1, 42}, io.ErrShortBuffer},
		{"empty native report", 2, 0, []byte{0, 0}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.size)
			n, err := copyReadReport(data, []byte{1, 42, 43}, tc.read)
			wantN := tc.read
			if wantN > tc.size {
				wantN = tc.size
			}
			if n != wantN || err != tc.err || !bytes.Equal(data, tc.want) {
				t.Fatalf("read = %d, %v, %v; want %d, %v, %v", n, err, data, wantN, tc.err, tc.want)
			}
		})
	}
}

func TestCopyReadReportRejectsImpossibleNativeLength(t *testing.T) {
	for _, read := range []int{-1, 4} {
		data := []byte{99, 99}
		if n, err := copyReadReport(data, []byte{1, 42, 43}, read); n != 0 || err == nil {
			t.Fatalf("read %d = %d, %v; want 0, error", read, n, err)
		}
		if !bytes.Equal(data, []byte{99, 99}) {
			t.Fatal("invalid native length modified caller data")
		}
	}
}

func TestInputReportWithoutNativeByteCount(t *testing.T) {
	data := make([]byte, 2)
	n, err := copyInputReport(data, []byte{1, 42, 43}, 0)
	if n != 0 || err != nil || !bytes.Equal(data, []byte{1, 42}) {
		t.Fatalf("input without byte count = %d, %v, %v", n, err, data)
	}
	// Feature reports and counted input reports retain actual-length semantics.
	n, err = copyInputReport(data, []byte{1, 42, 43}, 3)
	if n != len(data) || err != io.ErrShortBuffer {
		t.Fatalf("counted input = %d, %v; want %d, short buffer", n, err, len(data))
	}
}
