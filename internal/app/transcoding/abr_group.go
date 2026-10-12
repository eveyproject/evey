package transcoding

import (
	"strings"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// ABRGroup identifies a set of renditions a player may auto-switch between
// (its ABR ladder + any in-master fallback rung) WITHOUT reinitialising its
// video decoder. Two renditions in different groups cannot sit in the same
// switchable master: dropping from one to the other mid-buffer swaps the
// MediaCodec/SourceBuffer decoder, which ExoPlayer and hls.js do under
// starvation by freezing the picture with no error (a series episode, a H.264
// copy rung next to a GPU-HEVC transcode fallback, 2026-09-15). A rung in a
// different group must be reached by an EXPLICIT source change (teardown +
// reload at the same position), never by ABR.
//
// The dimensions are the ones that force a decoder reconfiguration:
//   - CodecFamily: h264 / hevc / av1 / ... (an H.264->HEVC switch is a new decoder).
//   - HDR: SDR vs PQ/HLG. An HDR rendition is 10-bit with a wide-gamut transfer;
//     an SDR one is 8-bit Rec.709. Switching between them reconfigures the
//     decoder and the display pipeline, so it is treated as a separate group.
//     HDR-ness also stands in for bit depth here: our transcode ladder emits
//     10-bit only on the HDR path and 8-bit otherwise.
type ABRGroup struct {
	CodecFamily string // canonical: "h264" | "hevc" | "av1" | ...
	HDR         bool   // true = PQ/HLG (10-bit wide gamut); false = SDR 8-bit
}

// NewABRGroup builds the group from a codec name (any spelling CanonicalVideoCodec
// folds - ffprobe name, encoder name, MIME) and an HLS VIDEO-RANGE value
// ("PQ"/"HLG" = HDR, "" = SDR). Do NOT pass an RFC-6381 CODECS string
// ("avc1.640028"): CanonicalVideoCodec does not parse the profile suffix and
// would fold it to "" - pass the source/output codec name instead.
func NewABRGroup(codec, videoRange string) ABRGroup {
	return ABRGroup{
		CodecFamily: transcoding.CanonicalVideoCodec(codec),
		HDR:         strings.TrimSpace(videoRange) != "",
	}
}

// SameAs reports whether a player may auto-switch between the two groups. A
// group with an empty CodecFamily (unrecognised codec) is never considered
// compatible with anything - the safe default is an explicit handoff.
func (g ABRGroup) SameAs(other ABRGroup) bool {
	if g.CodecFamily == "" || other.CodecFamily == "" {
		return false
	}
	return g.CodecFamily == other.CodecFamily && g.HDR == other.HDR
}
