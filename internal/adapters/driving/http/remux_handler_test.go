package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRemuxer stands in for the mkvgo adapter: it records how many times a
// remux was actually produced (to prove the on-disk cache collapses repeats)
// and writes a synthetic payload to the destination instead of running ffmpeg.
type fakeRemuxer struct {
	canRemux bool
	payload  []byte
	err      error
	calls    int
}

func (f *fakeRemuxer) CanRemux(string) bool { return f.canRemux }

func (f *fakeRemuxer) RemuxToMP4(_ context.Context, _, dst string) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(dst, f.payload, 0o644)
}

func newTestRemuxServe(t *testing.T, rmx Remuxer, libAccess LibraryAccessChecker) *remuxServe {
	t.Helper()
	return &remuxServe{
		libSvc:        &fakeMediaItemGetter{item: testMediaItem("lib1")},
		libraryAccess: libAccess,
		parental:      testFakeParentalChecker{},
		remuxer:       rmx,
		cacheDir:      filepath.Join(t.TempDir(), "remux"),
		baseURL:       "http://localhost",
	}
}

func doRemuxRequest(h *remuxServe, admin bool, rangeHdr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/remux.mp4", nil)
	req.SetPathValue("id", "m1")
	if rangeHdr != "" {
		req.Header.Set("Range", rangeHdr)
	}
	req = withHTTPClaims(req, testClaims(admin))
	w := httptest.NewRecorder()
	h.handle(w, req)
	return w
}

// An EBML source is remuxed once and served as a video/mp4 progressive file.
func TestRemux_HappyPath_ServesMP4(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: true, payload: []byte("FAKE-MP4-MOOV-FIRST")}
	h := newTestRemuxServe(t, rmx, allowAll())

	w := doRemuxRequest(h, false, "")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
	assert.Equal(t, "FAKE-MP4-MOOV-FIRST", w.Body.String())
	assert.Equal(t, 1, rmx.calls)
}

// The on-disk file IS the cache: a second request must serve the cached MP4
// without re-running the remux.
func TestRemux_CachedAfterFirstProduce(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: true, payload: []byte("DATA")}
	h := newTestRemuxServe(t, rmx, allowAll())

	require.Equal(t, http.StatusOK, doRemuxRequest(h, false, "").Code)
	require.Equal(t, http.StatusOK, doRemuxRequest(h, false, "").Code)

	assert.Equal(t, 1, rmx.calls, "second request must hit the on-disk cache, not remux again")
}

// Correctness guard: when the SOURCE changes (re-scan → new size+mtime
// fingerprint), the cached remux must be invalidated and re-produced, never
// served stale. Keying the cache on item.ID alone was the bug.
func TestRemux_SourceFingerprintChange_Reinvalidates(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: true, payload: []byte("DATA")}
	item := testMediaItem("lib1")
	item.File.Fingerprint = "100-200" // size-mtime, as the scanner sets it
	h := &remuxServe{
		libSvc:        &fakeMediaItemGetter{item: item},
		libraryAccess: allowAll(),
		parental:      testFakeParentalChecker{},
		remuxer:       rmx,
		cacheDir:      filepath.Join(t.TempDir(), "remux"),
		baseURL:       "http://localhost",
	}

	require.Equal(t, http.StatusOK, doRemuxRequest(h, false, "").Code)
	require.Equal(t, 1, rmx.calls)
	// Same fingerprint → cache hit, no re-remux.
	require.Equal(t, http.StatusOK, doRemuxRequest(h, false, "").Code)
	require.Equal(t, 1, rmx.calls)

	// Source replaced / re-scanned → fingerprint moves → must re-remux.
	item.File.Fingerprint = "999-888"
	require.Equal(t, http.StatusOK, doRemuxRequest(h, false, "").Code)
	assert.Equal(t, 2, rmx.calls, "a changed source fingerprint must re-remux, not serve the stale cache")
}

// fast-start + ServeContent means Range requests (seeking) return 206 with the
// requested byte slice.
func TestRemux_RangeRequest_PartialContent(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: true, payload: []byte("0123456789")}
	h := newTestRemuxServe(t, rmx, allowAll())

	w := doRemuxRequest(h, false, "bytes=2-5")

	require.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, "2345", w.Body.String())
}

// Only EBML sources can be repackaged; anything else is 415, never a remux.
func TestRemux_NonEBMLSource_Unsupported(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: false}
	h := newTestRemuxServe(t, rmx, allowAll())

	w := doRemuxRequest(h, false, "")

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	assert.Equal(t, 0, rmx.calls)
}

// IDOR guard: a profile without access to the item's library gets 404 (not an
// existence oracle) and the remux is never produced.
func TestRemux_LibraryAccessDenied_NotFound(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: true, payload: []byte("DATA")}
	h := newTestRemuxServe(t, rmx, denyAll())

	w := doRemuxRequest(h, false, "")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 0, rmx.calls, "denied access must never trigger a remux")
}

// A stream token authenticates ONE media: presenting media A's token on media
// B's remux is forbidden before any lookup. A remux that fails mid-produce is a
// 500 with no half-written output left for the next request to serve, and
// concurrent first requests produce once.
func TestRemux_TokenScopeAndProduceFailure(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: true, payload: []byte("DATA")}
	h := newTestRemuxServe(t, rmx, allowAll())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/remux.mp4", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{Token: "t", MediaItemID: "other"}))
	w := httptest.NewRecorder()
	h.handle(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, 0, rmx.calls)

	failing := &fakeRemuxer{canRemux: true, err: errors.New("mkvgo: broken cluster")}
	hf := newTestRemuxServe(t, failing, allowAll())
	w = doRemuxRequest(hf, false, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "broken cluster")
	entries, _ := os.ReadDir(hf.cacheDir)
	assert.Empty(t, entries, "no partial output survives a failed produce")

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { doRemuxRequest(h, false, "") })
	}
	wg.Wait()
	assert.Equal(t, 1, rmx.calls, "concurrent first requests share one produce")
}
