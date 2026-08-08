package photo

import (
	"encoding/binary"
	"testing"
)

// readExifData walks a JPEG's segment table by hand, on bytes that came from
// whatever file a user dropped into a watched folder. Every length in that walk
// is attacker-controlled: segment lengths, IFD entry counts, value offsets. A
// single missing bound is an index-out-of-range, and this runs inside the
// library scan worker, so the panic takes the scan down rather than skipping
// one bad photo.
//
// The unit tests cover the shapes we thought of. This covers the ones we did
// not: it asserts only that the parser RETURNS - never that it returns
// anything in particular - because on malformed input there is no right answer,
// only "does not crash".
//
// A mutation run put 14 of this package's survivors inside exif.go
// (2026-08-08), which is what prompted looking at it from the robustness side
// rather than the coverage side.
func FuzzReadExifData(f *testing.F) {
	// Well-formed starting points, so the fuzzer mutates something that already
	// reaches the interesting code rather than bouncing off the magic bytes.
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xD8})
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xD9})
	f.Add([]byte{0x89, 0x50, 0x4E, 0x47})
	f.Add(buildJPEGWithExif("2020:01:02 03:04:05", "Some Camera"))

	// A segment claiming to be far longer than the buffer: the classic way a
	// hand-rolled parser walks off the end.
	overlong := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	overlong = binary.BigEndian.AppendUint16(overlong, 0xFFFF)
	overlong = append(overlong, 'E', 'x', 'i', 'f', 0, 0)
	f.Add(overlong)

	f.Fuzz(func(t *testing.T, data []byte) {
		// No assertion on the values: garbage in, anything out is fine. The
		// contract under test is that the function comes back at all.
		takenAt, model := readExifData(data)
		if takenAt == nil && model == "" {
			return
		}
	})
}
