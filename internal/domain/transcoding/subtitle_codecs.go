package transcoding

import "strings"

// IsImageSubtitle reports whether the codec name represents a bitmap subtitle
// format. Bitmap subs (PGS, VOB, DVB, XSUB) can't be delivered as a WebVTT
// sidecar — they have to be burnt onto the video stream, which forces a video
// transcode. The single source of truth for bitmap detection: the decision
// layer, the HTTP handlers and the ffmpeg adapter all answer this question and
// MUST agree, or a selected bitmap track is silently dropped on one path while
// another burns it.
//
// Both the ffprobe canonical codec_name (hdmv_pgs_subtitle, dvd_subtitle,
// dvb_subtitle) and the common ffmpeg encoder aliases (pgssub, dvdsub, dvbsub)
// are matched: the prober stores the canonical name, but callers may pass
// either — missing dvb_subtitle in particular would silently route a DVB
// bitmap to the sidecar path, which can't render it.
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
