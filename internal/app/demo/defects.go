package demo

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/evey-project/evey/internal/pkg/demo"
)

// Matroska element IDs, as they appear in the file (length marker kept).
const (
	ebmlSegment = 0x18538067
	ebmlCues    = 0x1C53BB6B
	ebmlVoid    = 0xEC
)

// trailingBytes is what DefectTrailingBytes appends: enough for the scan to
// see, nothing a player reads.
const trailingBytes = 64 << 10

// applyDefect gives the file at path its manifest defect, unless it already
// carries it. It reports whether the file changed, so the library is rescanned.
func applyDefect(path string, d demo.Defect) (bool, error) {
	switch d {
	case demo.DefectNoSeekIndex:
		return voidCues(path)
	case demo.DefectTrailingBytes:
		return appendTrailing(path)
	default:
		return false, nil
	}
}

// segmentBounds returns where the Segment's children start and where its
// declared size ends. A Segment of unknown size has no end to compare with.
func segmentBounds(f *os.File) (start, end int64, err error) {
	id, n, err := readID(f, 0)
	if err != nil {
		return 0, 0, err
	}
	size, sn, err := readSize(f, int64(n))
	if err != nil {
		return 0, 0, err
	}
	if id != 0x1A45DFA3 {
		return 0, 0, errors.New("not an EBML file")
	}
	pos := int64(n+sn) + size
	id, n, err = readID(f, pos)
	if err != nil {
		return 0, 0, err
	}
	if id != ebmlSegment {
		return 0, 0, errors.New("no Segment after the EBML header")
	}
	size, sn, err = readSize(f, pos+int64(n))
	if err != nil {
		return 0, 0, err
	}
	if size < 0 {
		return 0, 0, errors.New("segment of unknown size")
	}
	start = pos + int64(n+sn)
	return start, start + size, nil
}

// voidCues rewrites the header of the Segment's Cues so the element becomes
// a Void of the same total length: an index-less file, byte for byte the same
// otherwise. The new header is 9 bytes (ID, 8-byte size), which every Cues
// element has room for.
func voidCues(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	pos, end, err := segmentBounds(f)
	if err != nil {
		return false, err
	}
	for pos < end {
		id, n, err := readID(f, pos)
		if err != nil {
			return false, err
		}
		size, sn, err := readSize(f, pos+int64(n))
		if err != nil {
			return false, err
		}
		if size < 0 {
			return false, fmt.Errorf("element 0x%X of unknown size at %d", id, pos)
		}
		total := int64(n+sn) + size
		if id == ebmlCues {
			if total < 9 {
				return false, fmt.Errorf("cues element of %d bytes at %d", total, pos)
			}
			hdr := make([]byte, 9)
			hdr[0], hdr[1] = ebmlVoid, 0x01
			for i, v := 0, total-9; i < 7; i++ {
				hdr[8-i] = byte(v >> (8 * i))
			}
			if _, err := f.WriteAt(hdr, pos); err != nil {
				return false, err
			}
			return true, f.Sync()
		}
		pos += total
	}
	return false, nil
}

// appendTrailing adds zeros past the Segment's declared end, once.
func appendTrailing(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, end, err := segmentBounds(f)
	if err != nil {
		return false, err
	}
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	if st.Size() > end {
		return false, nil
	}
	if _, err := f.WriteAt(make([]byte, trailingBytes), st.Size()); err != nil {
		return false, err
	}
	return true, f.Sync()
}

// readID reads an element ID at off, keeping its length marker.
func readID(r io.ReaderAt, off int64) (uint64, int, error) {
	v, n, err := readVint(r, off)
	if err != nil {
		return 0, 0, err
	}
	return v | 1<<(7*n), n, nil
}

// readSize reads an element size at off; -1 means unknown (all value bits set).
func readSize(r io.ReaderAt, off int64) (int64, int, error) {
	v, n, err := readVint(r, off)
	if err != nil {
		return 0, 0, err
	}
	if v == 1<<(7*n)-1 {
		return -1, n, nil
	}
	return int64(v), n, nil
}

// readVint reads an EBML variable-length integer and returns its value
// without the length marker, and its length in bytes.
func readVint(r io.ReaderAt, off int64) (uint64, int, error) {
	buf := make([]byte, 8)
	if _, err := r.ReadAt(buf[:1], off); err != nil {
		return 0, 0, err
	}
	n := 1
	for mask := byte(0x80); n <= 8 && buf[0]&mask == 0; mask >>= 1 {
		n++
	}
	if n > 8 {
		return 0, 0, fmt.Errorf("invalid vint at %d", off)
	}
	if n > 1 {
		if _, err := r.ReadAt(buf[1:n], off+1); err != nil {
			return 0, 0, err
		}
	}
	v := uint64(buf[0] & (0xFF >> n))
	for _, b := range buf[1:n] {
		v = v<<8 | uint64(b)
	}
	return v, n, nil
}
