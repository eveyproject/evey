package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// A season whose third episode is 5.1, served to a stereo device: the master
// leaves e3 out of the chain.
func stereoChainFixture(t *testing.T) (*hlsVODServe, *fakeConcatPackager, *chainStreamsGetter) {
	t.Helper()
	e1, e2, e3 := episode("e1", "s1"), episode("e2", "s1"), episode("e3", "s1")
	six := copyEligibleAV()
	ch := 6
	six[1].Channels = &ch
	fake := &chainStreamsGetter{
		items:       map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2, "e3": e3},
		streamsByID: map[string][]*domainlibrary.MediaStream{"e1": copyEligibleAV(), "e2": copyEligibleAV(), "e3": six},
		nextByID:    map[string]*domainlibrary.MediaItem{"e1": e2, "e2": e3},
	}
	pkg := &fakeConcatPackager{plan: &fakeConcatPlan{resources: map[string][]byte{
		"master.m3u8":   []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\ncc_playlist.m3u8\n"),
		"playlist.m3u8": []byte("#EXTM3U\n#EXT-X-MAP:URI=\"cc_p0/init.mp4\"\ncc_p0/seg00001.m4s\n"),
	}}}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.onDemandConcat = pkg
	h.libSvc = fake
	return h, pkg, fake
}

func hlsReq(pathAndQuery string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/media/e1/hls/"+pathAndQuery, nil)
	return r.WithContext(context.WithValue(r.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
}

func firstURI(t *testing.T, playlist string) string {
	t.Helper()
	for ln := range strings.SplitSeq(playlist, "\n") {
		if ln != "" && !strings.HasPrefix(ln, "#") {
			return ln
		}
	}
	t.Fatalf("no URI in %q", playlist)
	return ""
}

// The sub-resource request carries none of the client's capabilities, so
// re-deciding the chain from it put back the 5.1 episode the master had left
// out: the stereo device was served e3's segments. It now replays the chain
// the master stamped into its URIs.
func TestConcatSubResource_ReplaysTheChainItsMasterServed(t *testing.T) {
	h, pkg, _ := stereoChainFixture(t)
	e1 := episode("e1", "s1")
	master := hlsReq("master.m3u8?videoCodecs=h264&audioCodecs=aac&containers=mp4&gapless=1&maxAudioChannels=2")
	rp, err := h.probePlan(master, "e1", e1, copyEligibleAV())
	require.NoError(t, err)
	w := httptest.NewRecorder()
	require.True(t, h.serveConcatMaster(w, master, "e1", e1, rp, noopSpan()))
	want := []string{"/media/e1.mkv", "/media/e2.mkv"}
	require.Equal(t, want, pkg.lastSrcs, "fixture: the stereo master leaves e3 out")

	variant := firstURI(t, w.Body.String())
	require.Contains(t, variant, concatChainParam+"=2.", "the master stamps its chain")
	name, query, _ := strings.Cut(variant, "?")
	w2 := httptest.NewRecorder()
	h.serveConcatResource(w2, hlsReq(variant), "e1", name, e1, noopSpan())
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	assert.Equal(t, want, pkg.lastSrcs, "the playlist is the master's chain, not one re-decided from a bare request")
	assert.Contains(t, firstURI(t, w2.Body.String()), query, "segment URIs carry the stamp on")
}

// A chain changed since the master (an episode added between e1 and e2, or a
// file replaced) is not the chain the player was given: 410, it reloads.
func TestConcatSubResource_ChangedChainIsGone(t *testing.T) {
	h, pkg, fake := stereoChainFixture(t)
	e1 := episode("e1", "s1")
	stamp := concatChainStamp([]*domainlibrary.MediaItem{e1, episode("e2", "s1")})
	fake.items["e2"].File.Path = "/media/e2-replaced.mkv"
	calls := pkg.calls
	w := httptest.NewRecorder()
	h.serveConcatResource(w, hlsReq("cc_playlist.m3u8?"+concatChainParam+"="+stamp), "e1", concatPrefix+"playlist.m3u8", e1, noopSpan())
	assert.Equal(t, http.StatusGone, w.Code)
	assert.Equal(t, calls, pkg.calls, "no plan is built for a chain that is not the stamped one")

	w = httptest.NewRecorder()
	h.serveConcatResource(w, hlsReq("cc_playlist.m3u8?"+concatChainParam+"=7.forged"), "e1", concatPrefix+"playlist.m3u8", e1, noopSpan())
	assert.Equal(t, http.StatusGone, w.Code, "a stamp naming a longer chain than the season holds")
}

// A gapless viewer is a viewer: the background work that yields to playback
// sees the session.
func TestConcatSubResource_TouchesPlaybackActivity(t *testing.T) {
	h, _, _ := stereoChainFixture(t)
	h.playbackActivity = apptranscoding.NewPlaybackActivity(time.Minute)
	e1 := episode("e1", "s1")
	stamp := concatChainStamp([]*domainlibrary.MediaItem{e1, episode("e2", "s1")})
	w := httptest.NewRecorder()
	h.serveConcatResource(w, hlsReq("cc_playlist.m3u8?"+concatChainParam+"="+stamp), "e1", concatPrefix+"playlist.m3u8", e1, noopSpan())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, h.playbackActivity.ActiveSessionCount())
}
