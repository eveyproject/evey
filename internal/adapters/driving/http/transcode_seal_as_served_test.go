package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
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

// The fit survives the plan as served. A title decided "copy" whose copy rung
// is closed resolves a fitted transcode on the receiver's floor, then
// planAsServed re-resolves it under the vd=transcode pin, which never judges
// a line: the served master lost the fit and offered the whole ladder again,
// so the rescue reload rescued nothing on exactly the titles (variable frame
// rate, no keyframe index) that already took the transcode path.
func TestLineFit_FloorSurvivesPlanAsServed(t *testing.T) {
	h := lineHandler(t)
	item := lineItem(t, 20_000_000)
	vfr := copyEligibleAV()
	vfr[0].FrameRateMode = "vfr"
	fps := 30000.0 / 1001
	vfr[0].Framerate = &fps

	for name, req := range map[string]*http.Request{"floor": lineReq("", true), "line and floor": lineReq("6000", true)} {
		t.Run(name, func(t *testing.T) {
			first := planFor(t, h, req, item, vfr)
			require.Equal(t, "copy", first.Decision.VideoDecision, "premise: the codecs decide a copy")
			require.False(t, first.IsCopy, "premise: the copy rung is closed on a variable frame rate")
			require.Greater(t, len(first.Profiles), 1, "premise: a ladder with something to narrow")
			require.Equal(t, apptranscoding.LineFit{Single: true, Rung: 0}, first.Plan.Fit, "premise: the first resolution fits the floor")

			r, rp := h.planAsServed(req, "m1", item, vfr, first)
			assert.Equal(t, "transcode", r.URL.Query().Get("vd"))
			assert.Equal(t, apptranscoding.LineFit{Single: true, Rung: 0}, rp.Plan.Fit, "the fit rides the pin")
			master := masterOf(t, h, r, item, vfr, rp)
			assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"), "the served master is mono-variant")
			assert.Contains(t, master, "stream_0.m3u8")
			assert.Contains(t, master, "abr=floor")
		})
	}
}
