package photo

import (
	"encoding/binary"
	"strings"
	"time"
)

// readExifData extracts TakenAt and CameraModel from JPEG EXIF (APP1) bytes.
// Returns nil for takenAt if not found or not parseable. Silently ignores errors.
func readExifData(data []byte) (takenAt *time.Time, cameraModel string) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, ""
	}
	i := 2
	for i < len(data)-3 {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		if i+3 >= len(data) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		// A JPEG segment length counts its own two bytes, so the smallest legal
		// value is 2. A file declaring 0 or 1 puts `end` before the start of the
		// payload, and the `data[i+4:end]` slice below then panics with low >
		// high - found by fuzzing on "FF D8 FF E1 00 00". This parser runs in
		// the library scan worker, so that panic takes down the whole scan
		// instead of skipping one unreadable photo.
		if segLen < 2 {
			break
		}
		end := i + 2 + segLen
		if end > len(data) {
			break
		}
		if marker == 0xE1 {
			seg := data[i+4 : end]
			takenAt, cameraModel = parseExifSegment(seg)
			return takenAt, cameraModel
		}
		i = end
	}
	return nil, ""
}

// parseExifSegment parses a raw APP1 EXIF segment (after the 0xFFE1 marker and length).
// Expected layout: "Exif\0\0" + TIFF header + IFD0.
func parseExifSegment(seg []byte) (takenAt *time.Time, cameraModel string) {
	if len(seg) < 8 || string(seg[:6]) != "Exif\x00\x00" {
		return nil, ""
	}
	tiff := seg[6:]
	if len(tiff) < 8 {
		return nil, ""
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, ""
	}
	const entrySize = 12
	parseDT := func(entry []byte, dataType uint16, count int) *time.Time {
		s := readExifString(tiff, entry[8:12], dataType, count, order)
		if t, err := time.Parse("2006:01:02 15:04:05", s); err == nil {
			return &t
		}
		return nil
	}
	// walkIFD invokes fn for each 12-byte entry of the IFD at off; returns false
	// when the IFD header is out of range.
	walkIFD := func(off int, fn func(tag, dataType uint16, count int, entry []byte)) bool {
		if off < 0 || off+2 > len(tiff) {
			return false
		}
		n := int(order.Uint16(tiff[off : off+2]))
		pos := off + 2
		for range n {
			if pos+entrySize > len(tiff) {
				break
			}
			entry := tiff[pos : pos+entrySize]
			fn(order.Uint16(entry[0:2]), order.Uint16(entry[2:4]), int(order.Uint32(entry[4:8])), entry)
			pos += entrySize
		}
		return true
	}

	var dateTime, dateTimeOriginal *time.Time
	var exifIFDOffset int
	// Tag 0x0110 = Model, 0x0132 = DateTime (IFD0), 0x8769 = ExifIFDPointer.
	if !walkIFD(int(order.Uint32(tiff[4:8])), func(tag, dataType uint16, count int, entry []byte) {
		switch tag {
		case 0x0110:
			cameraModel = readExifString(tiff, entry[8:12], dataType, count, order)
		case 0x0132:
			dateTime = parseDT(entry, dataType, count)
		case 0x8769:
			exifIFDOffset = int(order.Uint32(entry[8:12]))
		}
	}) {
		return nil, ""
	}
	// DateTimeOriginal (the capture moment, tag 0x9003) lives in the Exif sub-IFD,
	// not IFD0. Prefer it: IFD0's DateTime (0x0132) can be a later edit time, which
	// would sort an edited photo by when it was edited, not when it was taken.
	if exifIFDOffset > 0 {
		walkIFD(exifIFDOffset, func(tag, dataType uint16, count int, entry []byte) {
			if tag == 0x9003 {
				dateTimeOriginal = parseDT(entry, dataType, count)
			}
		})
	}
	if dateTimeOriginal != nil {
		return dateTimeOriginal, cameraModel
	}
	return dateTime, cameraModel
}

// readExifString reads an ASCII string EXIF value. For short values (<=4 bytes) they're
// stored inline in the value offset field; otherwise they're at the offset into tiff.
func readExifString(tiff, valueField []byte, dataType uint16, count int, order binary.ByteOrder) string {
	if dataType != 2 || count <= 0 {
		return ""
	}
	var raw []byte
	if count <= 4 {
		raw = valueField[:count]
	} else {
		offset := int(order.Uint32(valueField))
		end := offset + count
		if offset < 0 || end > len(tiff) {
			return ""
		}
		raw = tiff[offset:end]
	}
	s := strings.TrimRight(string(raw), "\x00")
	return strings.TrimSpace(s)
}
