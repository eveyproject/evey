package transcoding

import "strings"

// CanonicalVideoCodec folds the common spellings/aliases of a video codec
// (client-advertised MIME/RFC forms, ffmpeg encoder names, ffprobe codec
// names) into the single canonical name used across the codebase - the
// ffprobe spelling the prober stores ("h264", "hevc", "av1", "vp9", "vp8",
// "mpeg2video", "vc1"). This is
// the ONE source of truth: decision.go, caps_override.go and hls_transcoder.go
// all fold through this table instead of keeping their own divergent copies.
//
// Returns "" for unrecognised input - callers pick their own fallback
// (lowercase passthrough, "any", empty-string-means-software, ...) rather
// than this function silently choosing one for them.
func CanonicalVideoCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "hevc", "h265", "x265", "libx265", "hvc1", "hev1":
		return "hevc"
	case "h264", "avc", "avc1", "x264", "libx264":
		return "h264"
	case "av1", "av01", "libaom-av1", "libsvtav1":
		return "av1"
	case "vp9", "vp09":
		return "vp9"
	// Decode-only classes (no encoder here): the legacy and web codecs the GPU
	// decode matrix probes, under their ffprobe codec_name.
	case "vp8", "vp08":
		return "vp8"
	case "mpeg2video", "mpeg2":
		return "mpeg2video"
	case "vc1", "wmv3", "wvc1", "wmva":
		return "vc1"
	}
	return ""
}
