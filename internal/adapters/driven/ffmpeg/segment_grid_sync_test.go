package ffmpeg

import (
	"strconv"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// TestSegmentGridConstantsInSync guards the HLS segment grid, which is defined
// by TWO constants in two packages that MUST hold the same value:
//
//   - hlsSegmentSeconds (this package) - the ffmpeg -hls_time cadence for every
//     video AND audio-rendition session, so the encoder cuts a segment every N s.
//   - apptranscoding.ManifestSegmentSeconds - the count the manifests advertise
//     (ceil(dur/grid)) for the transcode video ladder AND the audio renditions.
//
// They were "kept in sync" by comment only. If they ever diverged, a manifest
// would advertise more (or fewer) segments than the encoder produces: the video
// grid guard in serveSegment would mis-bound, and the audio tail - which has no
// count guard, only os.Stat + 503-retry - would loop on 503 forever instead of
// ending. This test turns that latent drift into a build failure.
func TestSegmentGridConstantsInSync(t *testing.T) {
	got, err := strconv.ParseFloat(hlsSegmentSeconds, 64)
	if err != nil {
		t.Fatalf("hlsSegmentSeconds %q is not numeric: %v", hlsSegmentSeconds, err)
	}
	if got != apptranscoding.ManifestSegmentSeconds {
		t.Fatalf("segment grid drift: ffmpeg hlsSegmentSeconds=%v but "+
			"apptranscoding.ManifestSegmentSeconds=%v - the -hls_time cadence and the "+
			"manifest segment count MUST match, or the stream tail mis-addresses/loops",
			got, apptranscoding.ManifestSegmentSeconds)
	}
}
