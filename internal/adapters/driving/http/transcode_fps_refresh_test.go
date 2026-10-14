package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A probe-version refresh can give a playing file the frame rate it had none
// of. The frame rate sets the segment grid, and the grid is in the decision
// fingerprint: the rendition URLs the player holds, sealed with the old
// fingerprint, are refused 410 stale_rendition instead of being served cut on
// another grid, and the master the player fetches again is served whole.
func TestFrameRateLearntMidPlayback_OldURLsRefusedNewMasterServed(t *testing.T) {
	h := lineHandler(t)
	item := lineItem(t, 4_000_000)
	req := lineReq("", false)
	variantOf := func(master string) string {
		for line := range strings.SplitSeq(master, "\n") {
			if strings.HasPrefix(line, "stream_0.m3u8") {
				return line
			}
		}
		t.Fatalf("no stream_0 variant in:\n%s", master)
		return ""
	}

	before := copyEligibleAV() // stored before the refresh: no frame rate
	rpBefore := planFor(t, h, req, item, before)
	heldURL := variantOf(masterOf(t, h, req, item, before, rpBefore))
	require.Contains(t, heldURL, "fp=", "the variant URL is sealed")

	after := copyEligibleAV()
	fps := 24000.0 / 1001
	after[0].Framerate = &fps
	rpAfter := planFor(t, h, req, item, after)
	require.NotEqual(t, rpBefore.FPHash, rpAfter.FPHash, "an exact frame rate puts a grid in the fingerprint")

	refused := func(url, currentFP string) (bool, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/"+url, nil)
		return h.refuseStaleFP(w, r, currentFP, "m1", "stream_0.m3u8"), w
	}
	// Until the refresh, the held URL is served.
	ok, _ := refused(heldURL, rpBefore.FPHash)
	assert.False(t, ok)
	// After it, the held URL is refused the way every player reloads on.
	ok, w := refused(heldURL, rpAfter.FPHash)
	require.True(t, ok, "a URL cut on the old grid must not be served on the new one")
	assert.Equal(t, http.StatusGone, w.Code)
	assert.Equal(t, "stale_rendition", w.Header().Get("X-Evey-Error"))
	// The master fetched again seals the new fingerprint, which is served.
	freshURL := variantOf(masterOf(t, h, req, item, after, rpAfter))
	ok, _ = refused(freshURL, rpAfter.FPHash)
	assert.False(t, ok)
}
