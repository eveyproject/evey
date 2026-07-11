package transcoding

import (
	"fmt"
	"math"
	"strings"
)

// buildCopyManifest renders the copy rung's HLS media playlist (VOD, fMP4) from
// the keyframe-aligned segment plan. Variable #EXTINF (keyframe boundaries are
// irregular, unlike the transcode ladder's fixed cadence), a shared #EXT-X-MAP
// init, and #EXT-X-ENDLIST (every boundary is known up front from the plan, so
// the client can seek anywhere immediately - the Jellyfin VOD model).
//
// Pure + deterministic in segs. The segment filenames (stream_0_NNN.m4s) match
// what the paced copy session writes and what serveSegment addresses by index,
// so the manifest and the on-disk segments never drift on naming.
func buildCopyManifest(segs []CopySegment) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:6\n")
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")

	// TARGETDURATION must be >= every #EXTINF, rounded up (RFC 8216 §4.3.3.1).
	var maxDurSec float64
	for _, s := range segs {
		if d := float64(s.DurationMs()) / 1000.0; d > maxDurSec {
			maxDurSec = d
		}
	}
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(maxDurSec)))
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
	b.WriteString("#EXT-X-MAP:URI=\"stream_0_init.mp4\"\n")
	for i, s := range segs {
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", float64(s.DurationMs())/1000.0)
		fmt.Fprintf(&b, "stream_0_%03d.m4s\n", i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}
