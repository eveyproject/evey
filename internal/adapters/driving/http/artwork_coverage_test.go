package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domainmeta "github.com/eveyproject/evey/internal/domain/metadata"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// --- shared helpers ---

// --- fakes for media handler tests ---

type fakeArtworkRepo struct {
	records map[string][]*domainmeta.ArtworkRecord
}

func (f *fakeArtworkRepo) GetByMediaItem(_ context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error) {
	return f.records[mediaItemID], nil
}

func (f *fakeArtworkRepo) UpdateSortOrder(_ context.Context, _ string, _ int) error { return nil }

func (f *fakeArtworkRepo) UpdateWebPPath(_ context.Context, _ string, _ string) error { return nil }

func TestMediaArtwork_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(mediaHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		"http://localhost",
		allowAll(),
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// An item that exists but has no artwork records returns 204 No Content (not
// 404): a benign "no poster yet" state must not surface as a browser console
// error for every posterless card. See writeNoArtwork in artwork_handler.go.
func TestMediaArtwork_NoRecords_204(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestMediaArtwork_IDOR_403(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
		&fakeArtworkRepo{},
		"http://localhost",
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- selectArtworkHandler ---

func TestSelectArtwork_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAdminMiddleware(verifier)(selectArtworkHandler(
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
		nil,
	))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSelectArtwork_NonAdmin_403(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	h := httpAdminMiddleware(verifier)(selectArtworkHandler(
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
		nil,
	))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSelectArtwork_MediaNotFound_404(t *testing.T) {
	h := selectArtworkHandler(
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{err: errTest},
		allowAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSelectArtwork_NoRecords_404(t *testing.T) {
	h := selectArtworkHandler(
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSelectArtwork_ArtworkIDNotFound_404(t *testing.T) {
	h := selectArtworkHandler(
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "other-art", MediaItemID: "m1", Type: "poster", SortOrder: 0}},
		}},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSelectArtwork_OK(t *testing.T) {
	h := selectArtworkHandler(
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {
				{ID: "art1", MediaItemID: "m1", Type: "poster", SortOrder: 1},
				{ID: "art2", MediaItemID: "m1", Type: "poster", SortOrder: 0},
			},
		}},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// The artwork every account sees is not a kids profile's to change; an
// unreadable profile is refused too (fail-closed), an adult one goes on.
func TestSelectArtwork_ParentalProfileRefused(t *testing.T) {
	kid := kidProfile(10)
	kid.ID = "p1"
	adult := &domainuser.Profile{ID: "p1", UserID: "u1"}
	for name, tc := range map[string]struct {
		getter *fakeProfileGetter
		want   int
	}{
		"kids profile":       {&fakeProfileGetter{profile: kid}, http.StatusForbidden},
		"unreadable profile": {&fakeProfileGetter{err: errTest}, http.StatusForbidden},
		"adult profile":      {&fakeProfileGetter{profile: adult}, http.StatusNotFound}, // no artwork records: past the gate
	} {
		h := selectArtworkHandler(&fakeArtworkRepo{}, &fakeMediaItemGetter{item: testMediaItem("lib1")}, allowAll(), tc.getter)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("artworkId", "art1")
		req = withHTTPClaims(req, testClaims(false))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: got %d, want %d (%s)", name, w.Code, tc.want, w.Body.String())
		}
	}
}
