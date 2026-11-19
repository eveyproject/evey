package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// The index is a map of the source, not packager output: an mkvgo upgrade
// must find the indexes built before it. Keyed on the version, every upgrade
// left each film to be walked again in full.
func TestSubtitleIndexCache_KeySurvivesAnMkvgoUpgrade(t *testing.T) {
	dir := t.TempDir()
	film := filepath.Join(dir, "film.mkv")
	require.NoError(t, os.WriteFile(film, []byte("not really matroska"), 0o644))
	c := newSubtitleIndexCache(dir)

	before := c.key(film)
	require.NotEmpty(t, before)
	prev := packagerVersion
	packagerVersion = func() string { return "v99.0.0" }
	t.Cleanup(func() { packagerVersion = prev })

	assert.Equal(t, before, c.key(film), "the index key must not follow the mkvgo version")
	assert.NotEqual(t, subtitleCache(c).keyFor(film, -1), c.key(film), "premise: the WebVTT key does follow it")
}

// A browser on a file the packager subtitles gets an index-only job: it never
// reads the WebVTT cache, but the packager reads the index, and without it
// each subtitle track followed walks the whole film. A native app keeps the
// full job.
func TestSubtitlePrewarm_BrowserOnPackagerSubtitlesQueuesTheIndex(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ua        string
		indexOnly bool
	}{
		{"browser", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36", true},
		{"native app", "EveyTV (Android TV 14; SHIELD)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := onDemandItem()
			item.ID, item.LibraryID = "m1", "lib1"
			streams := append(copyEligibleAV(), sub("subrip", "fre", false), sub("subrip", "fre", false))
			h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
			h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{resources: map[string][]byte{"master.m3u8": []byte(prodPlanMasterWithSubs)}}}
			p := newSubtitlePrewarmer(newSubtitleCache(t.TempDir()), newSubtitleIndexCache(t.TempDir()))
			h.subtitlePrewarm = p

			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/master.m3u8?videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4", nil)
			req.SetPathValue("id", "m1")
			req.SetPathValue("file", "master.m3u8")
			req.Header.Set("User-Agent", tc.ua)
			req = withHTTPClaims(req, testClaims(false))
			w := httptest.NewRecorder()
			h.serveHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, "mkvgo", w.Header().Get("X-Evey-Served-By"), "premise: the packager served the master")

			require.Len(t, p.jobs, 1)
			job := <-p.jobs
			assert.Equal(t, tc.indexOnly, job.indexOnly)
			assert.Len(t, job.tracks, 2)
		})
	}
}

// The index-only job has its own seen entry: a browser opening the film first
// must not cost a native app opening it later its extracted tracks.
func TestPrewarmer_IndexJobDoesNotShadowTheFullJob(t *testing.T) {
	p := newSubtitlePrewarmer(newSubtitleCache(t.TempDir()), subtitleIndexCache{})
	streams := []*domainlibrary.MediaStream{sub("subrip", "fre", false)}
	for range 3 {
		p.EnqueueIndex("m1", "/media/film.mkv", streams)
	}
	p.Enqueue("m1", "/media/film.mkv", streams, "")
	require.Len(t, p.jobs, 2)
	assert.True(t, (<-p.jobs).indexOnly)
	assert.False(t, (<-p.jobs).indexOnly)
}
