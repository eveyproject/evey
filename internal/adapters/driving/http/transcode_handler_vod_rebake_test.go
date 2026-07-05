package httpapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The copy-path media playlist emits bare URIs; serveVariant must re-bake the
// auth + decision suffix onto every media URI (and the EXT-X-MAP init) so the
// segment requests keep the token and the client caps (codecs, hdr) that drove
// the copy decision. Losing them 401s native players (no evey_hls cookie) and
// re-derives a TRANSCODE fingerprint that contradicts the master's copy CODECS.
func TestRebakeManifestURIs(t *testing.T) {
	manifest := strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-VERSION:6",
		"#EXT-X-PLAYLIST-TYPE:VOD",
		"#EXT-X-TARGETDURATION:11",
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXT-X-MAP:URI=\"stream_0_init.mp4\"",
		"#EXTINF:1.376,",
		"stream_0_000.m4s",
		"#EXTINF:10.427,",
		"stream_0_001.m4s",
		"#EXT-X-ENDLIST",
		"",
	}, "\n")
	suffix := "?hdr=1&token=tok&videoCodecs=h264%2Chevc"

	out := rebakeManifestURIs(manifest, suffix)

	assert.Contains(t, out, "#EXT-X-MAP:URI=\"stream_0_init.mp4"+suffix+"\"")
	assert.Contains(t, out, "\nstream_0_000.m4s"+suffix+"\n")
	assert.Contains(t, out, "\nstream_0_001.m4s"+suffix+"\n")
	// Tags other than MAP are untouched.
	assert.Contains(t, out, "#EXT-X-TARGETDURATION:11\n")
	assert.Contains(t, out, "#EXT-X-ENDLIST")

	// Empty suffix: bit-for-bit passthrough.
	assert.Equal(t, manifest, rebakeManifestURIs(manifest, ""))

	// URIs that already carry a query are left alone (defensive).
	pre := "#EXTM3U\nseg.m4s?token=x\n"
	assert.Equal(t, pre, rebakeManifestURIs(pre, suffix))
}
