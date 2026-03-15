package photo

import (
	"testing"
	"time"
)

// buildJPEGWithExif creates a minimal JPEG with an APP1 EXIF segment (little-endian TIFF).
func buildJPEGWithExif(datetime, model string) []byte {
	dtStr := datetime + "\x00"
	mdStr := model + "\x00"

	ifdOffset := 8
	numEntries := 2
	dataOffset := ifdOffset + 2 + numEntries*12 + 4
	dtOffset := dataOffset
	mdOffset := dataOffset + len(dtStr)
	totalLen := mdOffset + len(mdStr)

	tiff := make([]byte, totalLen)
	tiff[0] = 'I'
	tiff[1] = 'I'
	putU16le(tiff[2:], 42)
	putU32le(tiff[4:], uint32(ifdOffset))
	putU16le(tiff[8:], uint16(numEntries))

	e0 := tiff[10:]
	putU16le(e0[0:], 0x0132)
	putU16le(e0[2:], 2)
	putU32le(e0[4:], uint32(len(dtStr)))
	putU32le(e0[8:], uint32(dtOffset))

	e1 := tiff[22:]
	putU16le(e1[0:], 0x0110)
	putU16le(e1[2:], 2)
	putU32le(e1[4:], uint32(len(mdStr)))
	putU32le(e1[8:], uint32(mdOffset))

	putU32le(tiff[34:], 0)
	copy(tiff[dtOffset:], dtStr)
	copy(tiff[mdOffset:], mdStr)

	exifHeader := []byte("Exif\x00\x00")
	payload := append(exifHeader, tiff...)
	segLen := 2 + len(payload)
	app1 := []byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen & 0xFF)}
	app1 = append(app1, payload...)

	jpeg := []byte{0xFF, 0xD8}
	jpeg = append(jpeg, app1...)
	jpeg = append(jpeg, 0xFF, 0xD9)
	return jpeg
}

func putU16le(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putU32le(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func TestReadExifData_EmptyData(t *testing.T) {
	takenAt, model := readExifData([]byte{})
	if takenAt != nil || model != "" {
		t.Error("expected nil takenAt and empty model for empty data")
	}
}

func TestReadExifData_NonJPEG(t *testing.T) {
	takenAt, model := readExifData([]byte{0x89, 0x50, 0x4E, 0x47})
	if takenAt != nil || model != "" {
		t.Error("expected nil takenAt and empty model for non-JPEG data")
	}
}

func TestReadExifData_WithExif(t *testing.T) {
	jpeg := buildJPEGWithExif("2023:06:15 10:30:00", "Canon EOS R5")
	takenAt, model := readExifData(jpeg)
	if takenAt == nil {
		t.Fatal("expected non-nil takenAt")
	}
	expected := time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC)
	if !takenAt.Equal(expected) {
		t.Errorf("takenAt = %v, want %v", takenAt, expected)
	}
	if model != "Canon EOS R5" {
		t.Errorf("model = %q, want %q", model, "Canon EOS R5")
	}
}

func TestReadExifData_NoApp1(t *testing.T) {
	// JPEG with only APP0 (JFIF), no APP1
	jpeg := []byte{
		0xFF, 0xD8,
		0xFF, 0xE0, 0x00, 0x10,
		0x4A, 0x46, 0x49, 0x46, 0x00,
		0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
		0xFF, 0xD9,
	}
	takenAt, model := readExifData(jpeg)
	if takenAt != nil {
		t.Errorf("expected nil takenAt, got %v", takenAt)
	}
	if model != "" {
		t.Errorf("expected empty model, got %q", model)
	}
}
