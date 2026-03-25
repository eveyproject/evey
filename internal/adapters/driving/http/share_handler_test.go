package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appmedia "github.com/gravity-zero/evey/internal/app/media"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

// --- fake share link repo ---

type fakeShareLinkRepo struct {
	links     map[string]*shared.ShareLink
	byToken   map[string]*shared.ShareLink
	createErr error
	err       error
}

func newFakeShareLinkRepo() *fakeShareLinkRepo {
	return &fakeShareLinkRepo{
		links:   make(map[string]*shared.ShareLink),
		byToken: make(map[string]*shared.ShareLink),
	}
}

func (r *fakeShareLinkRepo) Create(_ context.Context, link *shared.ShareLink) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.links[link.ID] = link
	r.byToken[link.Token] = link
	return nil
}

func (r *fakeShareLinkRepo) GetByToken(_ context.Context, token string) (*shared.ShareLink, error) {
	if r.err != nil {
		return nil, r.err
	}
	l, ok := r.byToken[token]
	if !ok {
		return nil, errors.New("not found")
	}
	return l, nil
}

func (r *fakeShareLinkRepo) ListByUser(_ context.Context, _ string) ([]*shared.ShareLink, error) {
	return nil, r.err
}

func (r *fakeShareLinkRepo) ListAll(_ context.Context) ([]*shared.ShareLink, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]*shared.ShareLink, 0, len(r.links))
	for _, l := range r.links {
		out = append(out, l)
	}
	return out, nil
}

func (r *fakeShareLinkRepo) IncrementViewCount(_ context.Context, token string) error {
	if l, ok := r.byToken[token]; ok {
		l.ViewCount++
	}
	return nil
}

func (r *fakeShareLinkRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.links[id]; !ok {
		return errors.New("not found")
	}
	delete(r.links, id)
	return nil
}

func (r *fakeShareLinkRepo) DeleteExpired(_ context.Context) (int, error) { return 0, nil }

// --- helpers ---

type shareTestSetup struct {
	mux        *http.ServeMux
	adminToken string
	userToken  string
}

func newShareTestMux(t *testing.T, repo *fakeShareLinkRepo, mediaItems map[string]*domainlibrary.MediaItem, lac *fakeLibraryAccessChecker) shareTestSetup {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true, ProfileID: "prof-admin"})
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false, ProfileID: "prof-1"})

	shareSvc := appmedia.NewShareService(repo)
	mg := &fakeMediaGetter{items: mediaItems}

	mux := http.NewServeMux()
	registerShareRoutes(mux, gen, shareSvc, mg, lac)
	return shareTestSetup{mux: mux, adminToken: adminToken, userToken: userToken}
}

// --- share create ---

func TestShareCreate_NoAuth_401(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), map[string]*domainlibrary.MediaItem{"m1": testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(`{"durationHours":24}`))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestShareCreate_NonAdmin_403(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), map[string]*domainlibrary.MediaItem{"m1": testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(`{"durationHours":24}`))
	req.Header.Set("Authorization", "Bearer "+s.userToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestShareCreate_Success_201(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), map[string]*domainlibrary.MediaItem{"m1": testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(`{"durationHours":24}`))
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == nil || resp["token"] == "" {
		t.Error("expected token in response")
	}
}

func TestShareCreate_MediaNotFound_404(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), map[string]*domainlibrary.MediaItem{}, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(`{"durationHours":24}`))
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestShareCreate_InvalidBody_400(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), map[string]*domainlibrary.MediaItem{"m1": testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(`not json`))
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestShareCreate_DefaultDuration(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), map[string]*domainlibrary.MediaItem{"m1": testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

// --- share list all ---

func TestShareListAll_NoAuth_401(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), nil, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/share-links", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestShareListAll_Success_200(t *testing.T) {
	repo := newFakeShareLinkRepo()
	repo.links["l1"] = &shared.ShareLink{ID: "l1", Token: "tok1", MediaItemID: "m1", ExpiresAt: time.Now().Add(time.Hour)}
	repo.byToken["tok1"] = repo.links["l1"]
	s := newShareTestMux(t, repo, nil, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/share-links", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Errorf("expected 1 link, got %d", len(resp))
	}
}

// --- share revoke ---

func TestShareRevoke_NoAuth_401(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), nil, allowAll())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share-links/l1", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestShareRevoke_Success_204(t *testing.T) {
	repo := newFakeShareLinkRepo()
	repo.links["l1"] = &shared.ShareLink{ID: "l1"}
	s := newShareTestMux(t, repo, nil, allowAll())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share-links/l1", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestShareRevoke_NotFound_404(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), nil, allowAll())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share-links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- share validate (public, no auth needed) ---

func TestShareValidate_Success_200(t *testing.T) {
	repo := newFakeShareLinkRepo()
	link := &shared.ShareLink{ID: "l1", Token: "tok1", MediaItemID: "m1", ExpiresAt: time.Now().Add(time.Hour)}
	repo.links["l1"] = link
	repo.byToken["tok1"] = link
	items := map[string]*domainlibrary.MediaItem{
		"m1": {ID: "m1", Title: "Test Movie", FilePath: "/tmp/test.mkv", LibraryID: "lib1"},
	}
	s := newShareTestMux(t, repo, items, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok1", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["title"] != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got %v", resp["title"])
	}
}

func TestShareValidate_NotFound_404(t *testing.T) {
	s := newShareTestMux(t, newFakeShareLinkRepo(), nil, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/badtoken", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestShareValidate_Expired_410(t *testing.T) {
	repo := newFakeShareLinkRepo()
	link := &shared.ShareLink{ID: "l1", Token: "tok1", MediaItemID: "m1", ExpiresAt: time.Now().Add(-time.Hour)}
	repo.links["l1"] = link
	repo.byToken["tok1"] = link
	s := newShareTestMux(t, repo, nil, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok1", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Errorf("expected 410, got %d", w.Code)
	}
}

func TestShareValidate_MediaNotFound_404(t *testing.T) {
	repo := newFakeShareLinkRepo()
	link := &shared.ShareLink{ID: "l1", Token: "tok1", MediaItemID: "m1", ExpiresAt: time.Now().Add(time.Hour)}
	repo.links["l1"] = link
	repo.byToken["tok1"] = link
	s := newShareTestMux(t, repo, map[string]*domainlibrary.MediaItem{}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok1", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
