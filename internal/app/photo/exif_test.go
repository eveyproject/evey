package photo

import (
	"encoding/binary"
	"os"
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

func TestReadExifData_JustSOI(t *testing.T) {
	// Valid JPEG magic but only SOI, no markers to iterate
	takenAt, model := readExifData([]byte{0xFF, 0xD8, 0xFF, 0xD9})
	if takenAt != nil || model != "" {
		t.Error("expected nil/empty for JPEG with only SOI+EOI")
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

// --- stripJPEGApp1 ---

func TestStripJPEGApp1_NotJPEG(t *testing.T) {
	got := stripJPEGApp1([]byte{0x89, 0x50, 0x4E, 0x47})
	if got != nil {
		t.Error("expected nil for non-JPEG")
	}
}

func TestStripJPEGApp1_TooShort(t *testing.T) {
	got := stripJPEGApp1([]byte{0xFF})
	if got != nil {
		t.Error("expected nil for 1-byte data")
	}
}

func TestStripJPEGApp1_Empty(t *testing.T) {
	got := stripJPEGApp1(nil)
	if got != nil {
		t.Error("expected nil for nil data")
	}
}

func TestStripJPEGApp1_NoApp1(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0x01, 0x02}
	got := stripJPEGApp1(data)
	if got != nil {
		t.Error("expected nil when no APP1 present")
	}
}

func TestStripJPEGApp1_WithApp1(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB, // APP1 segment (len=4)
		0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD, // APP0 segment (len=4)
	}
	got := stripJPEGApp1(data)
	if got == nil {
		t.Fatal("expected non-nil when APP1 present")
	}
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d] = 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestStripJPEGApp1_StandaloneMarkers(t *testing.T) {
	// SOI + RST0 + APP1 -> strip APP1, keep RST0
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xD0, // RST0 (standalone)
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB, // APP1
	}
	got := stripJPEGApp1(data)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	want := []byte{0xFF, 0xD8, 0xFF, 0xD0}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
}

func TestStripJPEGApp1_EOIMarker(t *testing.T) {
	// SOI + EOI + APP1 -> EOI is standalone, APP1 stripped
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xD9, // EOI (standalone)
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB,
	}
	got := stripJPEGApp1(data)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	want := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
}

func TestStripJPEGApp1_TruncatedSegmentLength(t *testing.T) {
	// SOI + marker but not enough bytes for segment length
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00}
	got := stripJPEGApp1(data)
	if got != nil {
		t.Error("expected nil (no full APP1 to strip)")
	}
}

func TestStripJPEGApp1_SegmentLenBeyondData(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0xFF}
	got := stripJPEGApp1(data)
	if got != nil {
		t.Error("expected nil when segment extends beyond data")
	}
}

func TestStripJPEGApp1_NonMarkerByte(t *testing.T) {
	// SOI + non-0xFF byte -> treated as image data, appended as-is
	data := []byte{0xFF, 0xD8, 0x42, 0x43, 0x44}
	got := stripJPEGApp1(data)
	if got != nil {
		t.Error("expected nil (no APP1 found)")
	}
}

func TestStripJPEGApp1_MultipleApp1(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB, // APP1 #1
		0xFF, 0xE1, 0x00, 0x04, 0xCC, 0xDD, // APP1 #2
		0xFF, 0xE0, 0x00, 0x04, 0xEE, 0xFF, // APP0
	}
	got := stripJPEGApp1(data)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	// Both APP1 stripped, only SOI + APP0 remain
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xEE, 0xFF}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d] = 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

// --- stripExifIfJPEG ---

func TestStripExifIfJPEG_NonJPEGExtension(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.png"
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB}
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
	stripExifIfJPEG(path)
	after, _ := readFileBytes(path)
	// .png extension -> skipped, file unchanged
	if len(after) != len(data) {
		t.Errorf("file should be unchanged for .png, len changed from %d to %d", len(data), len(after))
	}
}

func TestStripExifIfJPEG_JPGExtension(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jpg"
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB,
		0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD,
	}
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
	stripExifIfJPEG(path)
	after, _ := readFileBytes(path)
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD}
	if len(after) != len(want) {
		t.Fatalf("after strip len = %d, want %d", len(after), len(want))
	}
}

func TestStripExifIfJPEG_JPEGExtension(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jpeg"
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xE1, 0x00, 0x04, 0xAA, 0xBB,
		0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD,
	}
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
	stripExifIfJPEG(path)
	after, _ := readFileBytes(path)
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xCC, 0xDD}
	if len(after) != len(want) {
		t.Fatalf("after strip len = %d, want %d", len(after), len(want))
	}
}

func TestStripExifIfJPEG_NonJPEGMagic(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jpg"
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
	stripExifIfJPEG(path)
	after, _ := readFileBytes(path)
	if len(after) != len(data) {
		t.Error("non-JPEG magic should leave file unchanged")
	}
}

func TestStripExifIfJPEG_TooShortFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jpg"
	if err := writeFile(path, []byte{0xFF}); err != nil {
		t.Fatal(err)
	}
	stripExifIfJPEG(path)
	after, _ := readFileBytes(path)
	if len(after) != 1 {
		t.Error("too-short file should be unchanged")
	}
}

func TestStripExifIfJPEG_MissingFile(t *testing.T) {
	// Should not panic
	stripExifIfJPEG("/nonexistent/path/test.jpg")
}

func TestStripExifIfJPEG_NoApp1_Unchanged(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jpg"
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0x01, 0x02}
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
	stripExifIfJPEG(path)
	after, _ := readFileBytes(path)
	if len(after) != len(data) {
		t.Error("file without APP1 should be unchanged")
	}
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// --- readExifString ---

func TestReadExifString_NonASCII(t *testing.T) {
	got := readExifString(nil, nil, 3, 5, nil)
	if got != "" {
		t.Errorf("expected empty for dataType!=2, got %q", got)
	}
}

func TestReadExifString_ZeroCount(t *testing.T) {
	got := readExifString(nil, nil, 2, 0, nil)
	if got != "" {
		t.Errorf("expected empty for zero count, got %q", got)
	}
}

func TestReadExifString_NegativeCount(t *testing.T) {
	got := readExifString(nil, nil, 2, -1, nil)
	if got != "" {
		t.Errorf("expected empty for negative count, got %q", got)
	}
}

func TestReadExifString_ShortInline(t *testing.T) {
	field := []byte{'A', 'B', 0x00, 0x00}
	got := readExifString(nil, field, 2, 3, nil)
	if got != "AB" {
		t.Errorf("got %q, want %q", got, "AB")
	}
}

func TestReadExifString_Exactly4Bytes(t *testing.T) {
	field := []byte{'A', 'B', 'C', 'D'}
	got := readExifString(nil, field, 2, 4, nil)
	if got != "ABCD" {
		t.Errorf("got %q, want %q", got, "ABCD")
	}
}

func TestReadExifString_LongOffset(t *testing.T) {
	tiff := make([]byte, 20)
	copy(tiff[10:], []byte("Canon\x00"))
	field := make([]byte, 4)
	putU32le(field, 10)
	got := readExifString(tiff, field, 2, 6, binary.LittleEndian)
	if got != "Canon" {
		t.Errorf("got %q, want %q", got, "Canon")
	}
}

func TestReadExifString_OffsetOutOfBounds(t *testing.T) {
	tiff := make([]byte, 5)
	field := make([]byte, 4)
	putU32le(field, 100)
	got := readExifString(tiff, field, 2, 6, binary.LittleEndian)
	if got != "" {
		t.Errorf("expected empty for OOB, got %q", got)
	}
}

// --- parseExifSegment ---

func TestParseExifSegment_TooShort(t *testing.T) {
	takenAt, model := parseExifSegment([]byte{0x01, 0x02})
	if takenAt != nil || model != "" {
		t.Error("expected nil/empty for too-short segment")
	}
}

func TestParseExifSegment_BadExifHeader(t *testing.T) {
	takenAt, model := parseExifSegment([]byte("NotExif\x00\x00Extra"))
	if takenAt != nil || model != "" {
		t.Error("expected nil/empty for bad exif header")
	}
}

func TestParseExifSegment_BadByteOrder(t *testing.T) {
	seg := make([]byte, 20)
	copy(seg[:6], "Exif\x00\x00")
	seg[6] = 'X'
	seg[7] = 'X'
	takenAt, model := parseExifSegment(seg)
	if takenAt != nil || model != "" {
		t.Error("expected nil/empty for unknown byte order")
	}
}

func TestParseExifSegment_ShortTIFF(t *testing.T) {
	// "Exif\0\0" + only 6 bytes of TIFF (needs 8)
	seg := []byte("Exif\x00\x00IIXXYY")
	takenAt, model := parseExifSegment(seg[:12])
	if takenAt != nil || model != "" {
		t.Error("expected nil/empty for short TIFF header")
	}
}

func TestParseExifSegment_IFDOffsetOutOfBounds(t *testing.T) {
	seg := make([]byte, 20)
	copy(seg[:6], "Exif\x00\x00")
	seg[6] = 'I'
	seg[7] = 'I'
	putU16le(seg[8:], 42)
	putU32le(seg[10:], 200) // IFD offset way beyond data
	takenAt, model := parseExifSegment(seg)
	if takenAt != nil || model != "" {
		t.Error("expected nil/empty for OOB IFD offset")
	}
}

func TestParseExifSegment_BigEndian(t *testing.T) {
	jpeg := buildJPEGWithExifBE("2024:01:15 08:00:00", "Nikon Z9")
	// Extract the APP1 segment payload (skip SOI + marker + length)
	seg := jpeg[4 : len(jpeg)-2] // skip FFD8 FFE1 len(2) and FFD9

	// Actually parse the segment content (after APP1 marker+length)
	takenAt, model := parseExifSegment(seg[2:]) // skip the 2-byte length prefix
	if takenAt == nil {
		t.Fatal("expected non-nil takenAt")
	}
	want := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	if !takenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v", takenAt, want)
	}
	if model != "Nikon Z9" {
		t.Errorf("model = %q, want Nikon Z9", model)
	}
}

func buildJPEGWithExifBE(datetime, model string) []byte {
	dtStr := datetime + "\x00"
	mdStr := model + "\x00"

	ifdOffset := 8
	numEntries := 2
	dataOffset := ifdOffset + 2 + numEntries*12 + 4
	dtOffset := dataOffset
	mdOffset := dataOffset + len(dtStr)
	totalLen := mdOffset + len(mdStr)

	tiff := make([]byte, totalLen)
	tiff[0] = 'M'
	tiff[1] = 'M'
	putU16be(tiff[2:], 42)
	putU32be(tiff[4:], uint32(ifdOffset))
	putU16be(tiff[8:], uint16(numEntries))

	e0 := tiff[10:]
	putU16be(e0[0:], 0x0132) // DateTime
	putU16be(e0[2:], 2)
	putU32be(e0[4:], uint32(len(dtStr)))
	putU32be(e0[8:], uint32(dtOffset))

	e1 := tiff[22:]
	putU16be(e1[0:], 0x0110) // Model
	putU16be(e1[2:], 2)
	putU32be(e1[4:], uint32(len(mdStr)))
	putU32be(e1[8:], uint32(mdOffset))

	putU32be(tiff[34:], 0)
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

func putU16be(b []byte, v uint16) {
	b[0] = byte(v >> 8)
	b[1] = byte(v)
}

func putU32be(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}
