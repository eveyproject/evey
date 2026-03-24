package photo

import (
	"encoding/binary"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
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
	ifdOffset := int(order.Uint32(tiff[4:8]))
	if ifdOffset+2 > len(tiff) {
		return nil, ""
	}
	numEntries := int(order.Uint16(tiff[ifdOffset : ifdOffset+2]))
	pos := ifdOffset + 2
	const entrySize = 12
	for range numEntries {
		if pos+entrySize > len(tiff) {
			break
		}
		entry := tiff[pos : pos+entrySize]
		tag := order.Uint16(entry[0:2])
		dataType := order.Uint16(entry[2:4])
		count := int(order.Uint32(entry[4:8]))
		// Tag 0x0110 = Model, 0x9003 = DateTimeOriginal, 0x0132 = DateTime
		switch tag {
		case 0x0110: // Model
			cameraModel = readExifString(tiff, entry[8:12], dataType, count, order)
		case 0x9003, 0x0132: // DateTimeOriginal or DateTime
			if takenAt == nil {
				s := readExifString(tiff, entry[8:12], dataType, count, order)
				if t, err := time.Parse("2006:01:02 15:04:05", s); err == nil {
					takenAt = &t
				}
			}
		}
		pos += entrySize
	}
	return takenAt, cameraModel
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

func imageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
