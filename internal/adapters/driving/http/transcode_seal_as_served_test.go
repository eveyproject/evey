package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
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

	cases := map[string]struct {
		req    *http.Request
		rung   int // -1: whatever rung the line picks, above the lowest
		reason string
	}{
		"floor":          {lineReq("", true), 0, "abr_floor_requested"},
		"line and floor": {lineReq("6000", true), 0, "abr_floor_requested"},
		"line only":      {lineReq("6000", false), -1, "client_line_exceeded"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			first := planFor(t, h, tc.req, item, vfr)
			require.Equal(t, "copy", first.Decision.VideoDecision, "premise: the codecs decide a copy")
			require.False(t, first.IsCopy, "premise: the copy rung is closed on a variable frame rate")
			require.Greater(t, len(first.Profiles), 1, "premise: a ladder with something to narrow")
			require.True(t, first.Plan.Fit.Single, "premise: the first resolution fits the plan")
			want := tc.rung
			if want < 0 {
				want = first.Plan.Fit.Rung
				require.Positive(t, want, "premise: a 6 Mbps line picks a rung above the lowest")
			}
			require.Equal(t, want, first.Plan.Fit.Rung)

			r, rp := h.planAsServed(tc.req, "m1", item, vfr, first)
			assert.Equal(t, "transcode", r.URL.Query().Get("vd"))
			assert.Equal(t, apptranscoding.LineFit{Single: true, Rung: want}, rp.Plan.Fit, "the fit rides the pin")
			assert.Contains(t, rp.Decision.TranscodeReasons, tc.reason, "the fit's reason rides along, the served cause is the viewer's reload")
			master := masterOf(t, h, r, item, vfr, rp)
			assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"), "the served master is mono-variant")
			variantName := fmt.Sprintf("stream_%d.m3u8", want)
			var variant string
			for line := range strings.SplitSeq(master, "\n") {
				if strings.HasPrefix(line, variantName) {
					variant = line
				}
			}
			require.NotEmpty(t, variant, "the fitted rung is the one offered")
			assert.Contains(t, variant, "fp="+rp.FPHash, "the master seals the fingerprint of the plan as served")
			vreq := withHTTPClaims(httptest.NewRequest("GET", "/api/v1/media/m1/hls/"+variant, nil), testClaims(false))
			assert.Equal(t, rp.FPHash, planFor(t, h, vreq, item, vfr).FPHash, "the variant resolves the sealed fingerprint")
		})
	}
}

// The DASH manifest seals the fingerprint its segments resolve to, as the
// HLS master does. A title decided "copy" by its codecs that the receiver's
// floor fits onto a re-encode is served under the vd=transcode pin; the
// manifest was declared from the plan before the pin, so every segment URI
// it emitted named the copy's fingerprint and was refused as stale.
func TestServeManifestMPD_SealsTheFingerprintItsSegmentsResolveTo(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, Bitrate: 4_000_000, Size: 1 << 20}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	fps := 25.0
	streams[0].Framerate = &fps
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	withAudioRenditions(h)
	h.copyFallbackRung = true

	const query = "videoCodecs=h264&audioCodecs=aac&containers=mp4&maxHeight=1080&maxAudioChannels=2&abr=floor"
	req := withHTTPClaims(httptest.NewRequest("GET", "/api/v1/media/m1/hls/manifest.mpd?"+query, nil), testClaims(false))
	first := planFor(t, h, req, item, streams)
	require.Equal(t, "copy", first.Decision.VideoDecision, "premise: the codecs decide a copy")
	require.False(t, first.IsCopy, "premise: the floor fits the plan onto a re-encode")

	w := doVODRequestQ(h, "m1", "manifest.mpd", query)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	i := strings.Index(body, `media="`)
	require.Positive(t, i, "a segment template")
	media := body[i+len(`media="`):]
	media = strings.ReplaceAll(media[:strings.IndexByte(media, '"')], "&amp;", "&")
	segment := strings.Replace(media, "$Number%03d$", "001", 1)
	sealed := segment[strings.Index(segment, "fp=")+3:]
	sealed = sealed[:strings.IndexAny(sealed+"&", "&")]
	require.NotEmpty(t, sealed)

	sreq := withHTTPClaims(httptest.NewRequest("GET", "/api/v1/media/m1/hls/"+segment, nil), testClaims(false))
	assert.Equal(t, sealed, planFor(t, h, sreq, item, streams).FPHash, "the segment resolves the sealed fingerprint")
	assert.NotEqual(t, first.FPHash, sealed, "the manifest is declared from the plan as served, not the copy it replaced")
}
