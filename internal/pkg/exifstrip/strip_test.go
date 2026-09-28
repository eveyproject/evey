package exifstrip

import (
	"bytes"
	"testing"
)

func TestStripJPEGAppExif_NotJPEG(t *testing.T) {
	if StripJPEGAppExif([]byte{0x89, 0x50, 0x4E, 0x47}) != nil {
		t.Error("expected nil for non-JPEG")
	}
}

func TestStripJPEGAppExif_Nil(t *testing.T) {
	if StripJPEGAppExif(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestStripJPEGAppExif_TooShort(t *testing.T) {
	if StripJPEGAppExif([]byte{0xFF}) != nil {
		t.Error("expected nil for 1-byte")
	}
}

func TestStripJPEGAppExif_NoExifSegments(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0x01, 0x02}
	if StripJPEGAppExif(data) != nil {
		t.Error("expected nil when no APP1/APP2")
	}
}

func TestStripJPEGAppExif_StripsAPP1(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB, // APP1
		0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD, // APP0
	}
	got := StripJPEGAppExif(data)
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestStripJPEGAppExif_StripsAPP2(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE2, 0x00, 0x04, 0xAA, 0xBB, // APP2
		0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD, // APP0
	}
	got := StripJPEGAppExif(data)
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestStripJPEGAppExif_StripsBothAPP1AndAPP2(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB, // APP1
		0xFF, 0xE2, 0x00, 0x04, 0xCC, 0xDD, // APP2
		0xFF, 0xE0, 0x00, 0x04, 0xEE, 0xFF, // APP0
	}
	got := StripJPEGAppExif(data)
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xEE, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestStripJPEGAppExif_MultipleAPP1(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB,
		0xFF, 0xE1, 0x00, 0x04, 0xCC, 0xDD,
		0xFF, 0xE0, 0x00, 0x04, 0xEE, 0xFF,
	}
	got := StripJPEGAppExif(data)
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xEE, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestStripJPEGAppExif_StandaloneMarkers(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xD0, // RST0
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB,
	}
	got := StripJPEGAppExif(data)
	want := []byte{0xFF, 0xD8, 0xFF, 0xD0}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

// The last restart marker (RST7 = 0xD7) is standalone like RST0: read as a
// length-prefixed segment it would swallow the EXIF block that follows and
// ship it (mutation 2026-09-28: only RST0 was tested).
func TestStripJPEGAppExif_LastRestartMarkerIsStandalone(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xD7, // RST7
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB,
	}
	got := StripJPEGAppExif(data)
	want := []byte{0xFF, 0xD8, 0xFF, 0xD7}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

// A file cut right after a 0xFF byte must not panic reading the marker past
// the end; the dangling byte of an already-truncated file is dropped.
func TestStripJPEGAppExif_TrailingLoneFF(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB, 0xFF}
	got := StripJPEGAppExif(data)
	want := []byte{0xFF, 0xD8}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestStripJPEGAppExif_NonMarkerByte(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0x42, 0x43, 0x44}
	if StripJPEGAppExif(data) != nil {
		t.Error("expected nil (no APP1/APP2)")
	}
}

func TestStripJPEGAppExif_TruncatedLength(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00}
	if StripJPEGAppExif(data) != nil {
		t.Error("expected nil for truncated segment")
	}
}

func TestStripJPEGAppExif_SegmentBeyondData(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0xFF}
	if StripJPEGAppExif(data) != nil {
		t.Error("expected nil when segment extends beyond data")
	}
}
