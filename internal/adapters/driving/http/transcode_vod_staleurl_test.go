package httpapi

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// THE 2026-07-15 RAINBOW, structural edition. The ladder is re-derived on
// EVERY request; when it changes shape between the master fetch and a segment
// fetch (a capability flap, an admission pin gone cold, a load-forced ceiling
// moving), `stream_2` silently maps to a DIFFERENT rendition. The player then
// concatenates another encode's bytes onto the init segment it already
// holds - the decoder paints coloured macroblocks, the audio (AAC) survives,
// and the server logs show nothing but 200s.
//
// The master now SEALS its decision hash into every URI it emits (fp=…). A
// variant/segment request whose fp no longer matches the hash of THIS moment
// is refused with 410 Gone: the player refetches the master and rebuilds
// coherently. Silent corruption becomes a clean reload.

var fpParamRE = regexp.MustCompile(`stream_0\.m3u8\?[^\s"]*fp=([0-9a-f]{16})`)

func doVODRequestQ(h *hlsVODServe, mediaID, file, rawQuery string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+mediaID+"/hls/v2/"+file+"?"+rawQuery, nil)
	req.SetPathValue("id", mediaID)
	req.SetPathValue("file", file)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	return w
}

func TestHLSVOD_StaleRenditionURLIsRefusedNotSilentlyRemapped(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	// A capability the test can flap, like a hevc_vaapi probe failing under
	// load or a recalibration landing mid-play would.
	capability := domaintranscoding.SafeFallback()
	capability.SoftwareMaxHeight = 720
	h.capability = func() domaintranscoding.HostCapabilitySnapshot { return (capability).Snapshot() }

	// The master seals its decision hash into the URIs it emits.
	w := doVODRequest(h, "m1", "master.m3u8")
	require.Equal(t, http.StatusOK, w.Code)
	m := fpParamRE.FindStringSubmatch(w.Body.String())
	require.NotNil(t, m, "the master must seal fp= into its variant URIs:\n%s", w.Body.String())
	sealedFP := m[1]

	// While the world is unchanged, the sealed URL serves normally.
	w = doVODRequestQ(h, "m1", "stream_0.m3u8", "fp="+sealedFP)
	assert.Equal(t, http.StatusOK, w.Code, "same fp, same ladder: must serve")

	// The world moves: the software ceiling drops, the ladder reshapes, the
	// decision hash changes.
	capability.SoftwareMaxHeight = 360
	w = doVODRequest(h, "m1", "master.m3u8")
	require.Equal(t, http.StatusOK, w.Code)
	m2 := fpParamRE.FindStringSubmatch(w.Body.String())
	require.NotNil(t, m2)
	require.NotEqual(t, sealedFP, m2[1], "test rig: the capability flip must reshape the ladder, or this test proves nothing")

	// The OLD sealed URLs are now lies: stream_0 no longer means what the
	// player's init segment was built for. Refuse them - never remap silently.
	w = doVODRequestQ(h, "m1", "stream_0.m3u8", "fp="+sealedFP)
	assert.Equal(t, http.StatusGone, w.Code, "a stale variant URL must be refused, not silently remapped")

	w = doVODRequestQ(h, "m1", "stream_0_000.m4s", "fp="+sealedFP)
	assert.Equal(t, http.StatusGone, w.Code, "a stale segment URL must be refused, not serve another rendition's bytes")

	w = doVODRequestQ(h, "m1", "stream_0_init.mp4", "fp="+sealedFP)
	assert.Equal(t, http.StatusGone, w.Code, "a stale init URL is the most dangerous of all - it defines the decoder's parameters")

	// The CURRENT fp serves fine - the refusal is about staleness, not the
	// mechanism.
	w = doVODRequestQ(h, "m1", "stream_0.m3u8", "fp="+m2[1])
	assert.Equal(t, http.StatusOK, w.Code)

	// And a request with NO fp (an old client, a cached URL from before this
	// feature) keeps being served exactly as today: degrade open, not closed.
	w = doVODRequest(h, "m1", "stream_0.m3u8")
	assert.Equal(t, http.StatusOK, w.Code, "no fp on the URL = pre-feature client: serve as before")
}
