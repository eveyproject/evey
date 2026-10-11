package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// segReq is a routed od_ segment request whose URL carries src.
func segReq(src string) *http.Request {
	r := onDemandReq()
	r.URL.RawQuery = "v=x&" + planSourceParam + "=" + src
	return r
}

func TestServeOnDemandResource_URLOfAnotherSourceIsGone(t *testing.T) {
	h := newCopyTestHandler(t)
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		sourceID:  "new",
		resources: map[string][]byte{"seg00001.m4s": []byte("new bytes")},
	}}
	h.onDemand = pkg

	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, segReq("old"), "m1", "od_seg00001.m4s", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusGone, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	w = httptest.NewRecorder()
	h.serveOnDemandResource(w, segReq("new"), "m1", "od_seg00001.m4s", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "new bytes", w.Body.String())
}

func TestServeOnDemandResource_ReplacedUnderThePlanReplaysOnce(t *testing.T) {
	stale := &fakeOnDemandPlan{sourceID: "old", refuse: true}
	fresh := &fakeOnDemandPlan{sourceID: "new", resources: map[string][]byte{"seg00001.m4s": []byte("new bytes")}}

	// A URL without src (a master from before the stamp) gets the new bytes.
	h := newCopyTestHandler(t)
	pkg := &fakeOnDemandPackager{plan: stale, replanned: fresh}
	h.onDemand = pkg
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_seg00001.m4s", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "new bytes", w.Body.String())
	assert.Equal(t, 1, pkg.replans)

	// A URL naming the old source: the replay finds another source, 410.
	pkg = &fakeOnDemandPackager{plan: stale, replanned: fresh}
	h.onDemand = pkg
	w = httptest.NewRecorder()
	h.serveOnDemandResource(w, segReq("old"), "m1", "od_seg00001.m4s", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusGone, w.Code)
	assert.Equal(t, 1, pkg.replans)
}

func TestServeOnDemandResource_ReplaysOnlyOnce(t *testing.T) {
	h := newCopyTestHandler(t)
	pkg := &fakeOnDemandPackager{
		plan:      &fakeOnDemandPlan{sourceID: "a", refuse: true},
		replanned: &fakeOnDemandPlan{sourceID: "b", refuse: true},
	}
	h.onDemand = pkg
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_seg00001.m4s", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 1, pkg.replans, "a second refusal is final")
}

func TestServeOnDemandResource_PlaylistURIsCarryTheSource(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		sourceID: "abc",
		resources: map[string][]byte{"playlist.m3u8": []byte(
			"#EXTM3U\n#EXT-X-MAP:URI=\"od_init.mp4\"\n#EXTINF:6,\nod_seg00001.m4s\n")},
	}}
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, segReq("abc"), "m1", "od_playlist.m3u8", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "od_seg00001.m4s?v=")
	assert.Contains(t, w.Body.String(), "&src=abc")
	assert.Contains(t, w.Body.String(), `URI="od_init.mp4?v=`)
}

func TestStampPlanSourceURIs(t *testing.T) {
	in := "#EXTM3U\n" +
		"#EXT-X-MEDIA:TYPE=AUDIO,URI=\"audio_0.m3u8?x=1\"\n" +
		"#EVEY-AUDIO-URI:URI=\"od_audio1.m3u8?audioTrack=0\"\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=1\n" +
		"od_playlist.m3u8?vh=1\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=2\n" +
		"stream_1.m3u8?vh=1\n"
	out := stampPlanSourceURIs(in, "abc")
	assert.Contains(t, out, `URI="od_audio1.m3u8?audioTrack=0&src=abc"`)
	assert.Contains(t, out, "od_playlist.m3u8?vh=1&src=abc\n")
	assert.Contains(t, out, `URI="audio_0.m3u8?x=1"`, "Evey's own renditions are not the plan's")
	assert.Contains(t, out, "stream_1.m3u8?vh=1\n")
	assert.Equal(t, in, stampPlanSourceURIs(in, ""), "an unstamped plan adds nothing")
}
