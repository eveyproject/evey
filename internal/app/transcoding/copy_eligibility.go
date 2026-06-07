package transcoding

import "strings"

// CopyRungEligible reports whether a source video stream may be served as a
// video-COPY (remux) rung. Scope is H.264 8-bit SDR, AND — the guard — a KNOWN
// level: without it the master cannot advertise an accurate avc1.PPCCLL, and a
// wrong CODECS makes Safari/hls.js reject the rendition. So an unknown level
// (level == nil) gates copy OFF; the title falls back to transcode until a
// re-probe backfills the level. This is the "no copy without level" guard the
// CODECS pipeline depends on.
func CopyRungEligible(codec string, level *int, tenBit bool) bool {
	if strings.ToLower(strings.TrimSpace(codec)) != "h264" {
		return false
	}
	if level == nil {
		return false // unknown level → no accurate CODECS → no copy
	}
	if tenBit {
		return false // 10-bit is out of the copy scope (HEVC/HDR territory)
	}
	return true
}

// CopyVideoCodecString returns the RFC-6381 "avc1.PPCCLL" to advertise for a
// video-COPY rung of this source stream, or "" when the stream is not
// copy-eligible (non-H.264, 10-bit, or unknown level). One call combining the
// guard with the codec string, so the manifest builder cannot advertise a copy
// rung without a level having gated it.
func CopyVideoCodecString(codec, profile string, level *int, tenBit bool) string {
	if !CopyRungEligible(codec, level, tenBit) {
		return ""
	}
	return avcCodecString(profile, *level)
}
