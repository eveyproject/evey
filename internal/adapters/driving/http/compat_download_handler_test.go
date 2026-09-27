package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// fakeCompatTranscoder stands in for ffmpeg: it writes a synthetic payload to
// the destination (or fails), and counts productions to prove the cache and
// singleflight collapse repeats. Runs on the handler's detached goroutine, so
// everything it touches is atomic.
type fakeCompatTranscoder struct {
	payload []byte
	err     error
	calls   atomic.Int32
}

func (f *fakeCompatTranscoder) TranscodeCompat(_ context.Context, _, dst string, _ int64, onProgress func(int)) error {
	f.calls.Add(1)
	if onProgress != nil {
		onProgress(420)
	}
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(dst, f.payload, 0o644)
}

// fakeCompatStreams answers the "does the file already carry a renderable
// track" question with a fixed codec list.
type fakeCompatStreams struct{ codecs []string }

func (f *fakeCompatStreams) GetMediaStreams(context.Context, string) ([]*domainlibrary.MediaStream, error) {
	var out []*domainlibrary.MediaStream
	for _, c := range f.codecs {
		out = append(out, &domainlibrary.MediaStream{StreamType: domainlibrary.Audio, Codec: c})
	}
	return out, nil
}

func newTestCompatServe(t *testing.T, tr CompatTranscoder, access LibraryAccessChecker) *compatServe {
	t.Helper()
	streams := &fakeCompatStreams{codecs: []string{"ac3"}}
	h := newCompatServe(&fakeMediaItemGetter{item: testMediaItem("lib1")}, streams, access, testFakeParentalChecker{}, nil, tr, t.TempDir(), "http://localhost")
	return h
}

func doCompatRequest(h http.HandlerFunc, method, path string, admin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(admin))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func decodeCompatStatus(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// waitCompatStatus polls the status endpoint until it reports want - the
// production runs on a detached goroutine, so "ready" is an eventually.
func waitCompatStatus(t *testing.T, h *compatServe, want string) map[string]any {
	t.Helper()
	var last map[string]any
	require.Eventually(t, func() bool {
		w := doCompatRequest(h.handleStatus, http.MethodGet, "/api/v1/media/m1/compat-download/status", false)
		last = decodeCompatStatus(t, w)
		return last["status"] == want
	}, 2*time.Second, 10*time.Millisecond, "status never reached %q (last: %v)", want, last)
	return last
}

func TestCompatDownload_PrepareProduceServe(t *testing.T) {
	tr := &fakeCompatTranscoder{payload: []byte("compat-mkv-bytes")}
	h := newTestCompatServe(t, tr, allowAll())

	// Nothing exists yet: serving answers 409 with the absent status.
	w := doCompatRequest(h.handleServe, http.MethodGet, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "absent", decodeCompatStatus(t, w)["status"])

	// Kick the production.
	w = doCompatRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusAccepted, w.Code)
	waitCompatStatus(t, h, "ready")
	assert.EqualValues(t, 1, tr.calls.Load())

	// Re-POSTing a ready item is a cheap 200, not a re-production.
	w = doCompatRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 1, tr.calls.Load())

	// The finished file serves as a ranged, named attachment.
	w = doCompatRequest(h.handleServe, http.MethodGet, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "compat-mkv-bytes", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/compat-download", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Range", "bytes=0-5")
	req = withHTTPClaims(req, testClaims(false))
	rec := httptest.NewRecorder()
	h.handleServe(rec, req)
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, "compat", rec.Body.String())
}

// The owner's invariant: the transcode runs when and ONLY when the device
// says it cannot play the file as it is. A client declaring it renders one of
// the file's own codecs gets "unnecessary" and zero NAS work.
func TestCompatDownload_UnnecessaryWhenClientRendersTheSource(t *testing.T) {
	tr := &fakeCompatTranscoder{payload: []byte("x")}
	h := newTestCompatServe(t, tr, allowAll()) // source audio: ac3

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/compat-download?renderable=aac,mp3,ac3", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.handlePrepare(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "unnecessary", decodeCompatStatus(t, w)["status"])
	assert.EqualValues(t, 0, tr.calls.Load())

	// The same client WITHOUT ac3 in its declaration does need the variant.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/compat-download?renderable=aac,mp3", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w = httptest.NewRecorder()
	h.handlePrepare(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
	waitCompatStatus(t, h, "ready")
	assert.EqualValues(t, 1, tr.calls.Load())
}

func TestCompatDownload_FailureIsReportedAndRetryable(t *testing.T) {
	tr := &fakeCompatTranscoder{err: errors.New("disk full")}
	h := newTestCompatServe(t, tr, allowAll())

	w := doCompatRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusAccepted, w.Code)
	failed := waitCompatStatus(t, h, "failed")
	assert.Contains(t, failed["error"], "disk full")

	// A failed production must not leave a half-written file behind.
	entries, err := os.ReadDir(filepath.Join(h.cacheDir))
	if err == nil {
		for _, e := range entries {
			assert.NotContains(t, e.Name(), ".tmp")
		}
	}

	// Re-POST clears the failure and tries again (the operator freed space).
	tr.err = nil
	tr.payload = []byte("second try")
	w = doCompatRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusAccepted, w.Code)
	waitCompatStatus(t, h, "ready")
	assert.EqualValues(t, 2, tr.calls.Load())
}

// panickingCompatTranscoder stands in for an encoder that panics mid-job.
type panickingCompatTranscoder struct{}

func (panickingCompatTranscoder) TranscodeCompat(context.Context, string, string, int64, func(int)) error {
	panic("encoder blew up")
}

// The production runs on a goroutine detached from the request: a panic there
// took the whole server down. It must end as a failed job instead.
func TestCompatDownload_PanicEndsAsAFailedJob(t *testing.T) {
	h := newTestCompatServe(t, panickingCompatTranscoder{}, allowAll())

	w := doCompatRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusAccepted, w.Code)
	failed := waitCompatStatus(t, h, "failed")
	assert.Contains(t, failed["error"], "encoder blew up")
}

// gatedCompatTranscoder blocks every production until the gate opens and
// records how many ran at once - the concurrency the cap under test bounds.
type gatedCompatTranscoder struct {
	payload []byte
	gate    chan struct{}
	running atomic.Int32
	maxSeen atomic.Int32
}

func (g *gatedCompatTranscoder) TranscodeCompat(_ context.Context, _, dst string, _ int64, _ func(int)) error {
	n := g.running.Add(1)
	for {
		seen := g.maxSeen.Load()
		if n <= seen || g.maxSeen.CompareAndSwap(seen, n) {
			break
		}
	}
	<-g.gate
	g.running.Add(-1)
	return os.WriteFile(dst, g.payload, 0o644)
}

// perIDMediaItemGetter hands every requested id its own item and source path,
// so each production targets a distinct cache file.
type perIDMediaItemGetter struct{}

func (perIDMediaItemGetter) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	return &domainlibrary.MediaItem{
		ID:        shared.MediaItemID(id),
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/tmp/" + id + ".mkv"},
	}, nil
}

// Distinct items must not each get their own ffmpeg: the singleflight only
// collapses repeats of the SAME item, so a season requested by several
// devices used to mean one production per episode, all at once, on the
// weak-NAS target. Beyond the cap they wait their turn, visibly "preparing".
func TestCompatDownload_ProductionsAreCapped(t *testing.T) {
	tr := &gatedCompatTranscoder{payload: []byte("x"), gate: make(chan struct{})}
	streams := &fakeCompatStreams{codecs: []string{"ac3"}}
	h := newCompatServe(perIDMediaItemGetter{}, streams, allowAll(), testFakeParentalChecker{}, nil, tr, t.TempDir(), "http://localhost")

	post := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+id+"/compat-download", nil)
		req.SetPathValue("id", id)
		req = withHTTPClaims(req, testClaims(false))
		w := httptest.NewRecorder()
		h.handlePrepare(w, req)
		return w
	}
	statusOf := func(id string) string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+id+"/compat-download/status", nil)
		req.SetPathValue("id", id)
		req = withHTTPClaims(req, testClaims(false))
		w := httptest.NewRecorder()
		h.handleStatus(w, req)
		s, _ := decodeCompatStatus(t, w)["status"].(string)
		return s
	}

	for _, id := range []string{"e1", "e2", "e3"} {
		assert.Equal(t, http.StatusAccepted, post(id).Code)
	}
	require.Eventually(t, func() bool { return tr.running.Load() == compatMaxConcurrentProductions },
		2*time.Second, 5*time.Millisecond)
	// Give an uncapped third production the time to prove itself running.
	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, compatMaxConcurrentProductions, tr.maxSeen.Load(), "more concurrent productions than the cap")
	for _, id := range []string{"e1", "e2", "e3"} {
		assert.Equal(t, "preparing", statusOf(id))
	}

	close(tr.gate)
	for _, id := range []string{"e1", "e2", "e3"} {
		require.Eventually(t, func() bool { return statusOf(id) == "ready" },
			2*time.Second, 10*time.Millisecond, "production %s never finished", id)
	}
	assert.EqualValues(t, compatMaxConcurrentProductions, tr.maxSeen.Load())
}

func TestCompatDownload_GuardsMirrorTheRawDownload(t *testing.T) {
	tr := &fakeCompatTranscoder{payload: []byte("x")}

	// Library access refused: no production must ever start. 404 (not 403) is
	// checkLibraryAccess's anti-enumeration contract - a denied library must
	// be indistinguishable from a missing item.
	denied := newTestCompatServe(t, tr, &fakeLibraryAccessChecker{allowed: false})
	w := doCompatRequest(denied.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 0, tr.calls.Load())

	// A stream token scoped to another media id is an IDOR attempt.
	h := newTestCompatServe(t, tr, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/compat-download", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{MediaItemID: "OTHER"}))
	rec := httptest.NewRecorder()
	h.handlePrepare(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.EqualValues(t, 0, tr.calls.Load())

	// A title the profile's parental rules hide is a 404 and never produced,
	// library access notwithstanding: the compat file is the film itself.
	blocked := newTestCompatServe(t, tr, allowAll())
	blocked.parental = testFakeParentalChecker{block: true}
	w = doCompatRequest(blocked.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 0, tr.calls.Load())
}
