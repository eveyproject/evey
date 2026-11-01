package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// The fingerprint a master seals into its variant URIs is the one those URIs
// resolve to. A variable-frame-rate source is decided "copy" by its codecs,
// but its copy rung is closed: the master serves a re-encode and pins
// vd=transcode, and it sealed the copy's fingerprint - every variant was then
// refused 410 stale_rendition, the master refetched forever, nothing played.
func TestMasterSealsTheFingerprintItsVariantsResolveTo(t *testing.T) {
	h := lineHandler(t)
	item := lineItem(t, 4_000_000)
	vfr := copyEligibleAV()
	vfr[0].FrameRateMode = "vfr"
	fps := 30000.0 / 1001
	vfr[0].Framerate = &fps

	cases := map[string][]*domainlibrary.MediaStream{"variable frame rate": vfr, "a real copy": copyEligibleAV()}
	for name, streams := range cases {
		t.Run(name, func(t *testing.T) {
			req := withHTTPClaims(httptest.NewRequest("GET", "/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264&audioCodecs=aac&maxHeight=1080&maxAudioChannels=2", nil), testClaims(false))
			r, rp := h.planAsServed(req, "m1", item, streams, planFor(t, h, req, item, streams))
			var variant string
			for line := range strings.SplitSeq(masterOf(t, h, r, item, streams, rp), "\n") {
				if strings.HasPrefix(line, "stream_0.m3u8") {
					variant = line
				}
			}
			require.NotEmpty(t, variant)
			vreq := withHTTPClaims(httptest.NewRequest("GET", "/api/v1/media/m1/hls/"+variant, nil), testClaims(false))
			assert.Contains(t, variant, "fp="+rp.FPHash)
			assert.Equal(t, rp.FPHash, planFor(t, h, vreq, item, streams).FPHash, "the variant must resolve to the sealed fingerprint")
		})
	}
}
