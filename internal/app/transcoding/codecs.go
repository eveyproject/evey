package transcoding

import "strings"

// IsImageSubtitle reports whether the codec name represents a bitmap subtitle
// format. Bitmap subs (PGS, VOB, DVB) can't be delivered as a WebVTT sidecar —
// they have to be burnt onto the video stream, which forces a video transcode.
// Used by the playback-decision and HLS pipelines to decide between the burn-in
// filter graph and the sidecar/passthrough path. This is the single bitmap-
// detection source of truth for the application layer.
//
// Both the ffprobe canonical codec_name (hdmv_pgs_subtitle, dvd_subtitle,
// dvb_subtitle) and the common ffmpeg encoder aliases (pgssub, dvdsub, dvbsub)
// are matched: the prober stores the canonical name, but callers may pass either
// — missing dvb_subtitle in particular would silently route a DVB bitmap to the
// sidecar path, which can't render it.
//
// Lives next to the rest of the codec-level domain helpers in this package so
// the HTTP layer doesn't need to reach into the ffmpeg adapter for it.
func IsImageSubtitle(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "hdmv_pgs_subtitle", "pgssub",
		"dvd_subtitle", "dvdsub",
		"dvb_subtitle", "dvbsub",
		"xsub":
		return true
	}
	return false
}
