package exifstrip

import (
	"bytes"
	"encoding/binary"
)

// StripJPEGAppExif returns JPEG bytes with all APP1 (0xFFE1), APP2 (0xFFE2),
// APP13 (0xFFED) and COM (0xFFFE) segments removed, stripping EXIF, XMP, ICC,
// IPTC and comment metadata.
// Returns nil if no such segments were found (no change needed).
// Non-JPEG input returns nil.
func StripJPEGAppExif(data []byte) []byte {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}
	var out bytes.Buffer
	out.Write(data[:2]) // SOI marker
	i := 2
	found := false
	for i < len(data)-1 {
		if data[i] != 0xFF {
			out.Write(data[i:])
			break
		}
		marker := data[i+1]
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			out.Write(data[i : i+2])
			i += 2
			continue
		}
		if i+3 >= len(data) {
			out.Write(data[i:])
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		end := i + 2 + segLen
		if end > len(data) {
			out.Write(data[i:])
			break
		}
		if marker == 0xE1 || marker == 0xE2 || marker == 0xED || marker == 0xFE {
			found = true
		} else {
			out.Write(data[i:end])
		}
		i = end
	}
	if !found {
		return nil
	}
	return out.Bytes()
}
