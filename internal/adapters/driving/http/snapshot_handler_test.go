package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

type fakeFrameExt struct {
	data       []byte
	err        error
	calledPath string
	calledPos  float64
	calledW    int
}

func (f *fakeFrameExt) ExtractFrame(_ context.Context, filePath string, pos float64, width int) ([]byte, error) {
	f.calledPath = filePath
	f.calledPos = pos
	f.calledW = width
	return f.data, f.err
}

func newSnapshotReq(t *testing.T, mediaID, query string) *http.Request {
	t.Helper()
	url := "/api/v1/media/" + mediaID + "/snapshot"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("id", mediaID)
	return req
}

func TestSnapshot_ParentalBlocked_NotFound(t *testing.T) {
	h := snapshotHandler(
		&fakeFrameExt{data: []byte("jpeg-data")},
		allowAll(), // library access granted…
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/x/video.mkv"}}},
		"http://localhost",
		alwaysBlockChecker{}, // …but parental blocks before any frame is extracted
	)
	req := newSnapshotReq(t, "m1", "position_ms=12500")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("parental-blocked snapshot must return 404 (existence hidden), got %d", w.Code)
	}
}

// A stream token is a capability scoped to one media item; replaying MOVIE1's
// token against MOVIE2's snapshot must be forbidden - this handler previously
// relied on library access only and leaked frames of any same-library media to
// a leaked token (pentest 2026-05-26).
func TestSnapshot_StreamTokenMediaMismatch_Forbidden(t *testing.T) {
	h := snapshotHandler(
		&fakeFrameExt{data: []byte("x")},
		allowAll(),
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m2", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/x.mkv"}}},
		"http://localhost", nil)

	req := newSnapshotReq(t, "m2", "position_ms=1000")
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey,
		&shared.StreamToken{UserID: "u1", MediaItemID: "m1"}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("a stream token for m1 must be forbidden on m2's snapshot, got %d", w.Code)
	}
}

func TestSnapshot_Success(t *testing.T) {
	fe := &fakeFrameExt{data: []byte("jpeg-data")}
	h := snapshotHandler(
		fe,
		allowAll(),
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/x/video.mkv"}}},
		"http://localhost", nil)

	req := newSnapshotReq(t, "m1", "position_ms=12500")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", w.Header().Get("Content-Type"))
	}
	if fe.calledPath != "/x/video.mkv" {
		t.Errorf("frame ext not called with file path, got %q", fe.calledPath)
	}
	if fe.calledPos != 12.5 {
		t.Errorf("expected pos 12.5s, got %f", fe.calledPos)
	}
	if fe.calledW != 640 {
		t.Errorf("expected width 640, got %d", fe.calledW)
	}
	if w.Body.String() != "jpeg-data" {
		t.Errorf("body mismatch: %s", w.Body.String())
	}
}

func TestSnapshot_NegativePositionClampedToZero(t *testing.T) {
	fe := &fakeFrameExt{data: []byte("x")}
	h := snapshotHandler(
		fe,
		allowAll(),
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/x.mkv"}}},
		"http://localhost", nil)

	req := newSnapshotReq(t, "m1", "position_ms=-5000")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if fe.calledPos != 0 {
		t.Errorf("expected pos 0 (clamped), got %f", fe.calledPos)
	}
}

func TestSnapshot_MediaNotFound(t *testing.T) {
	h := snapshotHandler(
		&fakeFrameExt{},
		allowAll(),
		&fakeMediaItemGetter{err: errors.New("not found")},
		"http://localhost", nil)

	req := newSnapshotReq(t, "ghost", "")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSnapshot_LibraryAccessDenied(t *testing.T) {
	h := snapshotHandler(
		&fakeFrameExt{data: []byte("x")},
		denyAll(),
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-private", File: &domainlibrary.MediaFile{Path: "/x.mkv"}}},
		"http://localhost", nil)

	req := newSnapshotReq(t, "m1", "")
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", ProfileID: "p1", IsAdmin: false})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// checkLibraryAccess returns 404 for non-admin without access
	if w.Code == http.StatusOK {
		t.Fatalf("expected non-200 on denied access, got 200")
	}
}

func TestSnapshot_ExtractError(t *testing.T) {
	h := snapshotHandler(
		&fakeFrameExt{err: errors.New("ffmpeg boom")},
		allowAll(),
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/x.mkv"}}},
		"http://localhost", nil)

	req := newSnapshotReq(t, "m1", "position_ms=0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
