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

func (f *fakeCompatTranscoder) TranscodeCompat(_ context.Context, _, dst string) error {
	f.calls.Add(1)
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(dst, f.payload, 0o644)
}

func newTestCompatServe(t *testing.T, tr CompatTranscoder, access LibraryAccessChecker) *compatServe {
	t.Helper()
	h := newCompatServe(&fakeMediaItemGetter{item: testMediaItem("lib1")}, access, testFakeParentalChecker{}, nil, tr, t.TempDir(), "http://localhost")
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

func decodeCompatStatus(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// waitCompatStatus polls the status endpoint until it reports want - the
// production runs on a detached goroutine, so "ready" is an eventually.
func waitCompatStatus(t *testing.T, h *compatServe, want string) map[string]string {
	t.Helper()
	var last map[string]string
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
}
