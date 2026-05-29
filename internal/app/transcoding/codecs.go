package transcoding

import "strings"

// IsImageSubtitle reports whether the codec name represents a bitmap subtitle
// format. Bitmap subs (PGS, VOB, DVB) can't be re-muxed alongside a transcode
// — they have to be burnt onto the video stream. Used by the HLS pipeline to
// decide between the burn-in filter graph and the sidecar/passthrough path.
//
// Lives next to the rest of the codec-level domain helpers in this package so
// the HTTP layer doesn't need to reach into the ffmpeg adapter for it.
func IsImageSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub":
		return true
	}
	return false
}
