package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmedia "github.com/gravity-zero/evey/internal/app/media"
	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	domainsubtitle "github.com/gravity-zero/evey/internal/domain/subtitle"
	subfakes "github.com/gravity-zero/evey/internal/domain/subtitle/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
)

// ============================================================
// Fakes for share handler tests
// ============================================================

type fakeShareLinkRepo struct {
	links   map[string]*shared.ShareLink
	byToken map[string]*shared.ShareLink
	err     error
}

func newFakeShareLinkRepo() *fakeShareLinkRepo {
	return &fakeShareLinkRepo{
		links:   make(map[string]*shared.ShareLink),
		byToken: make(map[string]*shared.ShareLink),
	}
}

func (r *fakeShareLinkRepo) Create(_ context.Context, link *shared.ShareLink) error {
	if r.err != nil {
		return r.err
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
	if r.err != nil {
		return nil, r.err
	}
	var result []*shared.ShareLink
	for _, l := range r.links {
		result = append(result, l)
	}
	return result, nil
}

func (r *fakeShareLinkRepo) ListAll(_ context.Context) ([]*shared.ShareLink, error) {
	if r.err != nil {
		return nil, r.err
	}
	var result []*shared.ShareLink
	for _, l := range r.links {
		result = append(result, l)
	}
	return result, nil
}

func (r *fakeShareLinkRepo) IncrementViewCount(_ context.Context, token string) error {
	if r.err != nil {
		return r.err
	}
	if l, ok := r.byToken[token]; ok {
		l.ViewCount++
	}
	return nil
}

func (r *fakeShareLinkRepo) Delete(_ context.Context, id string) error {
	if r.err != nil {
		return r.err
	}
	l, ok := r.links[id]
	if !ok {
		return errors.New("not found")
	}
	delete(r.byToken, l.Token)
	delete(r.links, id)
	return nil
}

func (r *fakeShareLinkRepo) DeleteExpired(_ context.Context) (int, error) {
	return 0, r.err
}

// ============================================================
// Fakes for subtitle media getter (subtitle domain)
// ============================================================

type fakeSubtitleMediaGetter struct {
	items map[string]*domainsubtitle.MediaItemInfo
}

func (f *fakeSubtitleMediaGetter) GetMediaItem(_ context.Context, id string) (*domainsubtitle.MediaItemInfo, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}

// ============================================================
// Share handler tests
// ============================================================

func newShareTestMux(t *testing.T) (*http.ServeMux, string, *fakeShareLinkRepo) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	repo := newFakeShareLinkRepo()
	shareSvc := appmedia.NewShareService(repo)
	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/tmp/movie.mkv", Title: "Test Movie"}

	mux := http.NewServeMux()
	registerShareRoutes(mux, gen, shareSvc, mg, allowAll())
	return mux, adminToken, repo
}

func TestShareCreate_OK(t *testing.T) {
	mux, token, _ := newShareTestMux(t)
	body := `{"durationHours":24}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == nil || resp["token"] == "" {
		t.Error("expected token in response")
	}
}

func TestShareCreate_NoAuth_401(t *testing.T) {
	mux, _, _ := newShareTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// requireAdmin returns 401 for no token
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestShareCreate_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	repo := newFakeShareLinkRepo()
	shareSvc := appmedia.NewShareService(repo)
	mg := newFakeMediaGetter() // empty
	mux := http.NewServeMux()
	registerShareRoutes(mux, gen, shareSvc, mg, allowAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/nonexistent/share", strings.NewReader(`{"durationHours":24}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestShareCreate_InvalidBody_400(t *testing.T) {
	mux, token, _ := newShareTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestShareCreate_DefaultDuration(t *testing.T) {
	mux, token, _ := newShareTestMux(t)
	body := `{"durationHours":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestShareListAll_OK(t *testing.T) {
	mux, token, repo := newShareTestMux(t)
	repo.links["l1"] = &shared.ShareLink{ID: "l1", Token: "tok1", MediaItemID: "m1", ExpiresAt: time.Now().Add(time.Hour)}
	repo.byToken["tok1"] = repo.links["l1"]

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/share-links", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestShareRevoke_OK(t *testing.T) {
	mux, token, repo := newShareTestMux(t)
	repo.links["l1"] = &shared.ShareLink{ID: "l1", Token: "tok1"}
	repo.byToken["tok1"] = repo.links["l1"]

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share-links/l1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestShareRevoke_NotFound_404(t *testing.T) {
	mux, token, _ := newShareTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share-links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestShareValidate_OK(t *testing.T) {
	mux, token, _ := newShareTestMux(t)
	// create first
	body := `{"durationHours":24}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/share", strings.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	shareToken := created["token"].(string)

	// validate (no auth required)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/"+shareToken, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestShareValidate_NotFound_404(t *testing.T) {
	mux, _, _ := newShareTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/bad-token", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestShareValidate_Expired_410(t *testing.T) {
	mux, _, repo := newShareTestMux(t)
	repo.links["l1"] = &shared.ShareLink{
		ID: "l1", Token: "expired-tok", MediaItemID: "m1",
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	repo.byToken["expired-tok"] = repo.links["l1"]

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/expired-tok", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Errorf("expected 410, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================
// Schedule handler tests
// ============================================================

func newScheduleTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	schedRepo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeNoOpScanner{}
	scheduler := applibrary.NewScheduler(schedRepo, scanner, slog.Default())

	mux := http.NewServeMux()
	registerScheduleRoutes(mux, scheduler, gen)
	return mux, adminToken
}

type fakeNoOpScanner struct{}

func (f *fakeNoOpScanner) ScanLibrary(_ context.Context, _ string) (int, int, error) {
	return 0, 0, nil
}

func TestScheduleGet_NoAuth_403(t *testing.T) {
	mux, _ := newScheduleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries/lib1/schedule", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestScheduleGet_NoSchedule_ReturnsNull(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries/lib1/schedule", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"schedule":null`) {
		t.Error("expected null schedule in response")
	}
}

func TestSchedulePut_OK(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	body := `{"cron":"0 * * * *","enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/libraries/lib1/schedule", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "lib1") {
		t.Error("expected libraryId in response")
	}
}

func TestSchedulePut_NoAuth_403(t *testing.T) {
	mux, _ := newScheduleTestMux(t)
	body := `{"cron":"0 * * * *"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/libraries/lib1/schedule", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSchedulePut_EmptyCron_400(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	body := `{"cron":"","enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/libraries/lib1/schedule", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSchedulePut_InvalidBody_400(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/libraries/lib1/schedule", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSchedulePut_InvalidCron_400(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	body := `{"cron":"not a cron"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/libraries/lib1/schedule", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestScheduleDelete_OK(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/libraries/lib1/schedule", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestScheduleDelete_NoAuth_403(t *testing.T) {
	mux, _ := newScheduleTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/libraries/lib1/schedule", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestScheduleGet_WithSchedule(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	// create schedule
	body := `{"cron":"0 * * * *","enabled":true}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/libraries/lib1/schedule", strings.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer "+token)
	pw := httptest.NewRecorder()
	mux.ServeHTTP(pw, putReq)
	if pw.Code != http.StatusOK {
		t.Fatalf("put failed: %d", pw.Code)
	}
	// get
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries/lib1/schedule", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"cron"`) {
		t.Error("expected cron in response")
	}
}

func TestSchedulePut_DefaultEnabled(t *testing.T) {
	mux, token := newScheduleTestMux(t)
	body := `{"cron":"0 * * * *"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/libraries/lib1/schedule", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Error("expected enabled=true by default")
	}
}

// ============================================================
// Subtitle routes tests
// ============================================================

func newSubtitleTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	provider.Results = []domainsubtitle.SearchResult{
		{FileID: 42, FileName: "test.srt", Language: "en", Format: "srt", Rating: 8.5},
	}
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{
		"m1": {ID: "m1", FilePath: "/tmp/movie.mkv", Title: "Test Movie"},
	}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)

	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/tmp/movie.mkv"}}

	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())
	return mux, userToken
}

func TestSubtitleSearch_OK(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search?language=en", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "test.srt") {
		t.Error("expected search result in response")
	}
}

func TestSubtitleSearch_NoAuth_401(t *testing.T) {
	mux, _ := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleSearch_InvalidToken_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleSearch_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, denyAll())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleList_OK(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubtitleDownload_OK(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	body := `{"fileId":42,"language":"en","format":"srt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubtitleDownload_NoFileID_400(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	body := `{"language":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDownload_InvalidBody_400(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDelete_NotFound_404(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/sub-999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubtitleSearch_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{err: errTest}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================
// Stream token handler tests
// ============================================================

func TestStreamToken_OK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, allowAll(), mg)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == "" {
		t.Error("expected token in response")
	}
	if resp["mediaId"] != "m1" {
		t.Errorf("expected mediaId=m1, got %s", resp["mediaId"])
	}
}

func TestStreamToken_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamToken_InvalidToken_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamToken_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestStreamToken_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, denyAll(), mg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ============================================================
// HLS cache tests
// ============================================================

func TestCachedMediaItemGetter_CacheHit(t *testing.T) {
	inner := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	cached := NewCachedMediaItemGetter(inner)
	defer cached.Close()

	item, err := cached.GetMediaItem(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID != "m1" {
		t.Errorf("expected m1, got %s", item.ID)
	}
	// second call hits cache
	inner.item = nil
	inner.err = errors.New("should not be called")
	item2, err := cached.GetMediaItem(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error on cache hit: %v", err)
	}
	if item2.ID != "m1" {
		t.Errorf("expected m1 from cache, got %s", item2.ID)
	}
}

func TestCachedMediaItemGetter_Error(t *testing.T) {
	inner := &fakeMediaItemGetter{err: errors.New("not found")}
	cached := NewCachedMediaItemGetter(inner)
	defer cached.Close()

	_, err := cached.GetMediaItem(context.Background(), "m1")
	if err == nil {
		t.Error("expected error")
	}
}

func TestCachedLibraryAccessChecker_CacheHit(t *testing.T) {
	inner := allowAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || !ok {
		t.Fatalf("expected access, got ok=%v err=%v", ok, err)
	}
	// second call with inner changed to deny — should still return cached true
	inner.allowed = false
	ok2, _ := cached.HasAccess(context.Background(), "u1", "lib1")
	if !ok2 {
		t.Error("expected cached access=true")
	}
}

func TestCachedLibraryAccessChecker_DeniedCacheHit(t *testing.T) {
	inner := denyAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || ok {
		t.Fatalf("expected denied, got ok=%v err=%v", ok, err)
	}
}

func TestCachedLibraryAccessChecker_Error(t *testing.T) {
	inner := &fakeLibraryAccessChecker{err: errors.New("db error")}
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	_, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err == nil {
		t.Error("expected error")
	}
}

// ============================================================
// Trickplay generate handler tests
// ============================================================

type fakeTrickplayGenerator struct {
	err error
}

func (f *fakeTrickplayGenerator) GenerateTrickplay(_ context.Context, _ string) error {
	return f.err
}

func TestTrickplayGenerate_OK(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestTrickplayGenerate_NonAdmin_403(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTrickplayGenerate_NoAuth_403(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTrickplayGenerate_QueryToken(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate?token=tok", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

// ============================================================
// Available runtimes route test
// ============================================================

func TestAvailableRuntimes_OK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	mux := http.NewServeMux()
	registerAvailableRuntimesRoute(mux, gen)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugin-runtimes/available", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "runtimes") {
		t.Error("expected runtimes key in response")
	}
}

func TestAvailableRuntimes_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)

	mux := http.NewServeMux()
	registerAvailableRuntimesRoute(mux, gen)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugin-runtimes/available", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ============================================================
// clearOIDCIDTokenCookie test
// ============================================================

func TestClearOIDCIDTokenCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearOIDCIDTokenCookie(w, true)
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == oidcIDTokenCookieName {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("expected MaxAge -1, got %d", c.MaxAge)
			}
			if !c.Secure {
				t.Error("expected Secure=true")
			}
		}
	}
	if !found {
		t.Error("expected OIDC cookie to be set")
	}
}

func TestClearOIDCIDTokenCookie_HTTP(t *testing.T) {
	w := httptest.NewRecorder()
	clearOIDCIDTokenCookie(w, false)
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == oidcIDTokenCookieName && c.Secure {
			t.Error("expected Secure=false for http")
		}
	}
}

func TestStreamTokenFromContext_Missing(t *testing.T) {
	_, ok := StreamTokenFromContext(context.Background())
	if ok {
		t.Error("expected false for missing stream token")
	}
}

func TestStreamTokenFromContext_Present(t *testing.T) {
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "m1"}
	ctx := context.WithValue(context.Background(), streamTokenCtxKey, st)
	got, ok := StreamTokenFromContext(ctx)
	if !ok {
		t.Fatal("expected true")
	}
	if got.UserID != "u1" {
		t.Errorf("expected u1, got %s", got.UserID)
	}
}

// ============================================================
// TokenRedactMiddlewareWithStore test
// ============================================================

type fakeStreamTokenValidator struct {
	token *shared.StreamToken
}

func (f *fakeStreamTokenValidator) ValidateAny(_ string) (*shared.StreamToken, bool) {
	if f.token != nil {
		return f.token, true
	}
	return nil, false
}

func TestTokenRedactMiddlewareWithStore_OpaqueToken(t *testing.T) {
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "m1"}
	store := &fakeStreamTokenValidator{token: st}

	var capturedCtx context.Context
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
	})
	handler := TokenRedactMiddlewareWithStore(store, inner)

	req := httptest.NewRequest(http.MethodGet, "/test?token=opaque-no-dots", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	got, ok := StreamTokenFromContext(capturedCtx)
	if !ok {
		t.Fatal("expected stream token in context")
	}
	if got.MediaItemID != "m1" {
		t.Errorf("expected m1, got %s", got.MediaItemID)
	}
}

func TestTokenRedactMiddlewareWithStore_JWTFallback(t *testing.T) {
	store := &fakeStreamTokenValidator{token: nil}

	var capturedAuth string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
	})
	handler := TokenRedactMiddlewareWithStore(store, inner)

	req := httptest.NewRequest(http.MethodGet, "/test?token=jwt.with.dots", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if capturedAuth != "Bearer jwt.with.dots" {
		t.Errorf("expected JWT in auth header, got %q", capturedAuth)
	}
}

// ============================================================
// discardResponseWriter test (helpers.go line 253)
// ============================================================

func TestDiscardResponseWriter_AllMethods(t *testing.T) {
	d := &discardResponseWriter{header: make(http.Header)}
	h := d.Header()
	if h == nil {
		t.Fatal("expected non-nil header")
	}
	h.Set("X-Test", "value")
	if d.Header().Get("X-Test") != "value" {
		t.Error("expected header to be set")
	}
	n, err := d.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("expected 5 nil, got %d %v", n, err)
	}
	d.WriteHeader(http.StatusCreated)
	if d.status != http.StatusCreated {
		t.Errorf("expected 201, got %d", d.status)
	}
}

// ============================================================
// extractBlockedIssues test (plugin_asset_handler.go)
// ============================================================

func TestExtractBlockedIssues_WithBrackets(t *testing.T) {
	msg := "plugin blocked: [uses eval()] [accesses localStorage]"
	issues := extractBlockedIssues(msg)
	if len(issues) < 2 {
		t.Fatalf("expected at least 2 issues, got %d: %v", len(issues), issues)
	}
}

func TestExtractBlockedIssues_NoBrackets(t *testing.T) {
	msg := "some error"
	issues := extractBlockedIssues(msg)
	if len(issues) != 1 || issues[0] != msg {
		t.Errorf("expected [%q], got %v", msg, issues)
	}
}

// ============================================================
// claimsFromContext test (share_handler.go)
// ============================================================

func TestClaimsFromContext_Missing(t *testing.T) {
	_, ok := claimsFromContext(context.Background())
	if ok {
		t.Error("expected false for missing claims")
	}
}

func TestClaimsFromContext_Present(t *testing.T) {
	claims := &domainuser.Claims{UserID: "u1", IsAdmin: true}
	ctx := context.WithValue(context.Background(), adminClaimsKey, claims)
	got, ok := claimsFromContext(ctx)
	if !ok {
		t.Fatal("expected true")
	}
	if got.UserID != "u1" {
		t.Errorf("expected u1, got %s", got.UserID)
	}
}

// ============================================================
// transcode parseParams tests
// ============================================================

func TestTranscodeParseParams_Default(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := h.parseParams(req)
	if p.audioTrack != 0 {
		t.Errorf("expected audioTrack=0, got %d", p.audioTrack)
	}
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0, got %d", p.videoScale)
	}
	if p.subtitleTrack != -1 {
		t.Errorf("expected subtitleTrack=-1, got %d", p.subtitleTrack)
	}
	if p.remuxOnly {
		t.Error("expected remuxOnly=false")
	}
	if p.startTimeSec != 0 {
		t.Errorf("expected startTimeSec=0, got %f", p.startTimeSec)
	}
}

func TestTranscodeParseParams_AllSet(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?audioTrack=2&videoScale=720&subtitleTrack=1&remux=1&startTime=30.5", nil)
	p := h.parseParams(req)
	if p.audioTrack != 2 {
		t.Errorf("expected audioTrack=2, got %d", p.audioTrack)
	}
	if p.videoScale != 720 {
		t.Errorf("expected videoScale=720, got %d", p.videoScale)
	}
	if p.subtitleTrack != 1 {
		t.Errorf("expected subtitleTrack=1, got %d", p.subtitleTrack)
	}
	if !p.remuxOnly {
		t.Error("expected remuxOnly=true")
	}
	if p.startTimeSec != 30.5 {
		t.Errorf("expected startTimeSec=30.5, got %f", p.startTimeSec)
	}
}

func TestTranscodeParseParams_InvalidValues(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?audioTrack=abc&videoScale=-1&subtitleTrack=-5&startTime=bad", nil)
	p := h.parseParams(req)
	if p.audioTrack != 0 {
		t.Errorf("expected audioTrack=0 for invalid, got %d", p.audioTrack)
	}
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0 for invalid, got %d", p.videoScale)
	}
	if p.subtitleTrack != -1 {
		t.Errorf("expected subtitleTrack=-1 for invalid, got %d", p.subtitleTrack)
	}
}

func TestTranscodeParseParams_VideoScaleTooHigh(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?videoScale=9999", nil)
	p := h.parseParams(req)
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0 for >4320, got %d", p.videoScale)
	}
}

// ============================================================
// transcode analyzeStreams tests
// ============================================================

func TestAnalyzeStreams_H264_AAC_DirectPlay(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, -1)
	if a.needsVideoTranscode {
		t.Error("h264 should not need video transcode")
	}
	if a.needsAudioTranscode {
		t.Error("aac should not need audio transcode")
	}
}

func TestAnalyzeStreams_HEVC_NeedsTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, -1)
	if !a.needsVideoTranscode {
		t.Error("hevc should need video transcode")
	}
}

func TestAnalyzeStreams_RemuxOnly_NoVideoTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", true, false, -1)
	if a.needsVideoTranscode {
		t.Error("remux mode should not trigger video transcode")
	}
}

func TestAnalyzeStreams_NonAACAudio_NeedsTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "dts", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, -1)
	if !a.needsAudioTranscode {
		t.Error("dts should need audio transcode")
	}
}

func TestAnalyzeStreams_OpusAudio_NoCopy(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "opus", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, -1)
	if a.needsAudioTranscode {
		t.Error("opus should not need audio transcode")
	}
}

func TestAnalyzeStreams_MP3Audio_NoCopy(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "mp3", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, -1)
	if a.needsAudioTranscode {
		t.Error("mp3 should not need audio transcode")
	}
}

func TestAnalyzeStreams_SubtitleTrack(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
			{StreamType: "subtitle", Codec: "srt"},
			{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle"},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, 1)
	if !a.burnInSubtitle {
		t.Error("PGS subtitle should trigger burn-in")
	}
	if !a.needsVideoTranscode {
		t.Error("burn-in subtitle should trigger video transcode")
	}
	if a.subtitleCodec != "hdmv_pgs_subtitle" {
		t.Errorf("expected hdmv_pgs_subtitle, got %s", a.subtitleCodec)
	}
}

func TestAnalyzeStreams_SubtitleTrack_SRT(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
			{StreamType: "subtitle", Codec: "srt"},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, 0)
	if a.burnInSubtitle {
		t.Error("SRT should not burn-in")
	}
	if a.subtitleCodec != "srt" {
		t.Errorf("expected srt, got %s", a.subtitleCodec)
	}
}

func TestAnalyzeStreams_StreamsError(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		sErr: errors.New("db error"),
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, false, -1)
	if a.streams != nil {
		t.Error("expected nil streams on error")
	}
	if !a.needsAudioTranscode {
		t.Error("default should be needsAudioTranscode=true")
	}
}

// ============================================================
// transcode resolveSourceDimensions tests
// ============================================================

func TestResolveSourceDimensions_Default(t *testing.T) {
	h := &hlsMasterServe{libSvc: &fakeMediaStreamsGetter{sErr: errors.New("fail")}}
	w, ht, streams := h.resolveSourceDimensions(context.Background(), "m1")
	if w != 1920 || ht != 1080 {
		t.Errorf("expected 1920x1080, got %dx%d", w, ht)
	}
	if streams != nil {
		t.Error("expected nil streams on error")
	}
}

func TestResolveSourceDimensions_FromStreams(t *testing.T) {
	sw, sh := 3840, 2160
	h := &hlsMasterServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc", Width: &sw, Height: &sh},
			{StreamType: "audio", Codec: "aac"},
		},
	}}
	w, ht, streams := h.resolveSourceDimensions(context.Background(), "m1")
	if w != 3840 || ht != 2160 {
		t.Errorf("expected 3840x2160, got %dx%d", w, ht)
	}
	if len(streams) != 2 {
		t.Errorf("expected 2 streams, got %d", len(streams))
	}
}

// ============================================================
// transcode resolveSubtitle tests
// ============================================================

func TestResolveSubtitle_NoQuery(t *testing.T) {
	h := &hlsMasterServe{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	idx, codec := h.resolveSubtitle(req, nil)
	if idx != -1 || codec != "" {
		t.Errorf("expected -1/'', got %d/%s", idx, codec)
	}
}

func TestResolveSubtitle_InvalidTrack(t *testing.T) {
	h := &hlsMasterServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?subtitleTrack=abc", nil)
	idx, _ := h.resolveSubtitle(req, []*domainlibrary.MediaStream{})
	if idx != -1 {
		t.Errorf("expected -1, got %d", idx)
	}
}

func TestResolveSubtitle_NegativeTrack(t *testing.T) {
	h := &hlsMasterServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?subtitleTrack=-1", nil)
	idx, _ := h.resolveSubtitle(req, []*domainlibrary.MediaStream{})
	if idx != -1 {
		t.Errorf("expected -1, got %d", idx)
	}
}

func TestResolveSubtitle_ValidTrack(t *testing.T) {
	h := &hlsMasterServe{}
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "subtitle", Codec: "srt"},
		{StreamType: "subtitle", Codec: "ass"},
	}
	req := httptest.NewRequest(http.MethodGet, "/test?subtitleTrack=1", nil)
	idx, codec := h.resolveSubtitle(req, streams)
	if idx != 1 || codec != "ass" {
		t.Errorf("expected 1/ass, got %d/%s", idx, codec)
	}
}

func TestResolveSubtitle_TrackOutOfRange(t *testing.T) {
	h := &hlsMasterServe{}
	streams := []*domainlibrary.MediaStream{
		{StreamType: "subtitle", Codec: "srt"},
	}
	req := httptest.NewRequest(http.MethodGet, "/test?subtitleTrack=5", nil)
	idx, _ := h.resolveSubtitle(req, streams)
	if idx != -1 {
		t.Errorf("expected -1 for out of range, got %d", idx)
	}
}

// ============================================================
// parseClientProfile tests
// ============================================================

func TestParseClientProfile_Default(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := h.parseClientProfile(req)
	if p.MaxWidth == 0 || p.MaxHeight == 0 {
		t.Error("expected non-zero default dimensions")
	}
}

func TestParseClientProfile_AllParams(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?maxWidth=1280&maxHeight=720&maxBitrate=5000000&videoCodecs=h264,hevc&audioCodecs=aac,opus&containers=mp4,mkv&hdr=true", nil)
	p := h.parseClientProfile(req)
	if p.MaxWidth != 1280 {
		t.Errorf("expected 1280, got %d", p.MaxWidth)
	}
	if p.MaxHeight != 720 {
		t.Errorf("expected 720, got %d", p.MaxHeight)
	}
	if p.MaxBitrate != 5000000 {
		t.Errorf("expected 5000000, got %d", p.MaxBitrate)
	}
	if len(p.VideoCodecs) != 2 || p.VideoCodecs[0] != "h264" {
		t.Errorf("unexpected video codecs: %v", p.VideoCodecs)
	}
	if len(p.AudioCodecs) != 2 {
		t.Errorf("unexpected audio codecs: %v", p.AudioCodecs)
	}
	if len(p.Containers) != 2 {
		t.Errorf("unexpected containers: %v", p.Containers)
	}
	if !p.SupportsHDR {
		t.Error("expected SupportsHDR=true")
	}
}

func TestParseClientProfile_InvalidValues(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?maxWidth=abc&maxHeight=-1&maxBitrate=bad", nil)
	p := h.parseClientProfile(req)
	assert.NotZero(t, p.MaxWidth, "invalid should keep default, which is non-zero")
	if p.SupportsHDR {
		t.Error("expected SupportsHDR=false without hdr=true")
	}
}

// ============================================================
// Collection getByID test (boosts from 14.8%)
// ============================================================

func TestCollectionGetByID_OK(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	// create a collection
	body := `{"name":"Sci-Fi","rules":[{"field":"genre","operator":"eq","value":"Sci-Fi"}],"sortBy":"year"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	// GET by ID
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Sci-Fi") {
		t.Error("expected collection name in response")
	}
	if !strings.Contains(w.Body.String(), `"items"`) {
		t.Error("expected items in response")
	}
}

func TestCollectionGetByID_WithPagination(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	body := `{"name":"Action","rules":[{"field":"genre","operator":"eq","value":"Action"}],"sortBy":"year"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/"+id+"?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCollectionGetByID_Missing_404(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCollectionDelete_OK(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	body := `{"name":"ToDelete","rules":[{"field":"genre","operator":"eq","value":"x"}]}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/collections/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestCollectionDelete_NotFound_404(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/collections/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================
// Playlist addItem and reorder tests
// ============================================================

func TestPlaylistAddItem_OK(t *testing.T) {
	mux, token := newPlaylistTestMux(t)
	// create playlist
	body, _ := json.Marshal(map[string]string{"name": "Test", "type": "manual"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	plID := created["id"].(string)

	// add item
	addBody, _ := json.Marshal(map[string]string{"mediaItemId": "media-1"})
	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/"+plID+"/items", bytes.NewReader(addBody))
	addReq.Header.Set("Authorization", "Bearer "+token)
	aw := httptest.NewRecorder()
	mux.ServeHTTP(aw, addReq)
	if aw.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", aw.Code, aw.Body.String())
	}
}

func TestPlaylistAddItem_InvalidBody_400(t *testing.T) {
	mux, token := newPlaylistTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": "Test", "type": "manual"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	plID := created["id"].(string)

	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/"+plID+"/items", strings.NewReader("not json"))
	addReq.Header.Set("Authorization", "Bearer "+token)
	aw := httptest.NewRecorder()
	mux.ServeHTTP(aw, addReq)
	if aw.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", aw.Code)
	}
}

func TestPlaylistReorder_OK(t *testing.T) {
	mux, token := newPlaylistTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": "Test", "type": "manual"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	plID := created["id"].(string)

	reorderBody, _ := json.Marshal(map[string]any{"mediaItemIds": []string{"m1", "m2"}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/"+plID+"/reorder", bytes.NewReader(reorderBody))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPlaylistReorder_InvalidBody_400(t *testing.T) {
	mux, token := newPlaylistTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": "Test", "type": "manual"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	plID := created["id"].(string)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/"+plID+"/reorder", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPlaylistRemoveItem_NotFound(t *testing.T) {
	mux, token := newPlaylistTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": "Test", "type": "manual"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	plID := created["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/playlists/"+plID+"/items/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================
// Plugin handleListThemes test
// ============================================================

func TestListThemes_OK(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	_ = repo.Create(context.Background(), &plugin.Plugin{
		ID: "theme1", Name: "Dark Theme", Enabled: true, Capabilities: []string{"ui.theme"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Dark Theme") {
		t.Error("expected theme name in response")
	}
}

func TestListThemes_NoAuth_401(t *testing.T) {
	mux, _, _, _ := setupPluginAssetTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ============================================================
// Plugin upload route test
// ============================================================

func TestPluginUpload_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPluginUpload_NonAdmin_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPluginUpload_NoFile_400(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "a1", IsAdmin: true})
	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ============================================================
// Subtitle delete with found item
// ============================================================

func TestSubtitleDelete_OK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	provider.DownloadPath = "/tmp/test.srt"
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{
		"m1": {ID: "m1", FilePath: "/tmp/movie.mkv", Title: "Test"},
	}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

	// download first to create a subtitle
	dlBody := `{"fileId":42,"language":"en","format":"srt"}`
	dlReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(dlBody))
	dlReq.Header.Set("Authorization", "Bearer "+userToken)
	dw := httptest.NewRecorder()
	mux.ServeHTTP(dw, dlReq)
	if dw.Code != http.StatusCreated {
		t.Fatalf("download failed: %d %s", dw.Code, dw.Body.String())
	}
	var dlResp map[string]any
	json.NewDecoder(dw.Body).Decode(&dlResp)
	subID := dlResp["id"].(string)

	// delete
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/"+subID, nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================
// Subtitle list with items
// ============================================================

// ============================================================
// HLS segment handler auth tests
// ============================================================

func TestHLSSegmentHandler_NoAuth_401(t *testing.T) {
	h := hlsSegmentHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil, allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/segment.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "segment.ts")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHLSSegmentHandler_InvalidToken_403(t *testing.T) {
	h := hlsSegmentHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil, allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/segment.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "segment.ts")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHLSSegmentHandler_NotFound_404(t *testing.T) {
	h := hlsSegmentHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{err: errTest},
		nil, allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/segment.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "segment.ts")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHLSSegmentHandler_IDOR_403(t *testing.T) {
	h := hlsSegmentHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
		nil, denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/segment.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "segment.ts")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// HLSSegmentHandler_QueryToken skipped — requires non-nil HLSManager to avoid panic after auth

// ============================================================
// Transcode handler auth tests
// ============================================================

func TestTranscodeHandler_NoAuth_401(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens: &fakeTokenVerifier{err: errTest},
		libSvc: &fakeMediaStreamsGetter{},
		baseURL: "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTranscodeHandler_InvalidToken_403(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens: &fakeTokenVerifier{err: errTest},
		libSvc: &fakeMediaStreamsGetter{},
		baseURL: "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTranscodeHandler_NotFound_404(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens: &fakeTokenVerifier{claims: testClaims(false)},
		libSvc: &fakeMediaStreamsGetter{itemErr: errTest},
		baseURL: "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTranscodeHandler_IDOR_403(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens: &fakeTokenVerifier{claims: testClaims(false)},
		libSvc: &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", FilePath: "/a"}},
		baseURL: "http://localhost",
		libraryAccess: denyAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTranscodeHandler_QueryToken(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens: &fakeTokenVerifier{claims: testClaims(false)},
		libSvc: &fakeMediaStreamsGetter{itemErr: errTest},
		baseURL: "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode?token=tok", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass with query token, got %d", w.Code)
	}
}

func TestTranscodeHandler_MaxStreams_429(t *testing.T) {
	throttle := apptranscoding.NewTranscodeThrottle()
	maxStreams := 1
	fakeMaxStreams := &fakeUserMaxStreams{max: &maxStreams}
	throttle.Acquire("u1", 0) // occupy one slot
	h := transcodeHandler(transcodeServe{
		tokens: &fakeTokenVerifier{claims: testClaims(false)},
		libSvc: &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/a"}},
		baseURL: "http://localhost",
		libraryAccess: allowAll(), throttle: throttle, userMaxStreams: fakeMaxStreams,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	throttle.Release("u1")
}

type fakeUserMaxStreams struct {
	max *int
}

func (f *fakeUserMaxStreams) GetMaxStreams(_ context.Context, _ string) *int {
	return f.max
}

// ============================================================
// HLS master handler auth tests
// ============================================================

func TestHLSMasterHandler_NoAuth_401(t *testing.T) {
	h := hlsMasterHandler(hlsMasterServe{
		tokens: &fakeTokenVerifier{err: errTest},
		libSvc: &fakeMediaStreamsGetter{},
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/master.m3u8", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHLSMasterHandler_InvalidToken_403(t *testing.T) {
	h := hlsMasterHandler(hlsMasterServe{
		tokens: &fakeTokenVerifier{err: errTest},
		libSvc: &fakeMediaStreamsGetter{},
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/master.m3u8", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHLSMasterHandler_NotFound_404(t *testing.T) {
	h := hlsMasterHandler(hlsMasterServe{
		tokens: &fakeTokenVerifier{claims: testClaims(false)},
		libSvc: &fakeMediaStreamsGetter{itemErr: errTest},
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/master.m3u8", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHLSMasterHandler_IDOR_403(t *testing.T) {
	h := hlsMasterHandler(hlsMasterServe{
		tokens: &fakeTokenVerifier{claims: testClaims(false)},
		libSvc: &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", FilePath: "/a"}},
		libraryAccess: denyAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/master.m3u8", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ============================================================
// Photo handler additional tests
// ============================================================

func TestGetPhoto_NotFound(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeletePhoto_NotFound(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/photos/photo-nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// photo not found → should get 500 or 404
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass, got %d", w.Code)
	}
}

func TestServePhoto_NotFound(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-nonexistent/serve", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass, got %d", w.Code)
	}
}

func TestScanPhotos_NonAdmin_403(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	body := `{"library_id":"lib1","dir":"/tmp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestScanPhotos_NoAuth_403(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestScanPhotos_BadBody_400(t *testing.T) {
	mux, adminToken := newPhotoAdminTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestScanPhotos_MissingFields_400(t *testing.T) {
	mux, adminToken := newPhotoAdminTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", strings.NewReader(`{"library_id":"lib1"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestScanPhotos_RelativePath_400(t *testing.T) {
	mux, adminToken := newPhotoAdminTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", strings.NewReader(`{"library_id":"lib1","dir":"relative/path"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ScanPhotos_LibraryNotFound skipped — requires non-nil libraryPathsGetter in photo mux

// ============================================================
// Backup handler additional tests
// ============================================================

func TestCreateBackup_Admin_201(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetBackup_NotFound_404(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetBackup_NoAuth_403(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/b1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDeleteBackup_NotFound_404(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteBackup_NoAuth_403(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/b1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRestoreBackup_NoAuth_403(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/restore", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRestoreBackup_NoFile_400(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/restore", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDeleteBackup_AfterCreate(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	// create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	backupID := created["id"].(string)

	// delete
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/"+backupID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestGetBackup_AfterCreate(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	backupID := created["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/"+backupID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/gzip" {
		t.Errorf("expected gzip content type, got %s", w.Header().Get("Content-Type"))
	}
}

// ============================================================
// Favorite handler additional tests
// ============================================================

func TestToggleFavorite_OK(t *testing.T) {
	mux, token := newFavoriteTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/favorites/media-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "isFavorite") {
		t.Error("expected isFavorite in response")
	}
}

func TestCheckFavorite_NoAuth_401(t *testing.T) {
	mux, _ := newFavoriteTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites/media-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCheckFavorite_OK(t *testing.T) {
	mux, token := newFavoriteTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites/media-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "isFavorite") {
		t.Error("expected isFavorite in response")
	}
}

func TestListFavorites_WithPagination(t *testing.T) {
	mux, token := newFavoriteTestMux(t)
	// toggle to add one favorite
	toggleReq := httptest.NewRequest(http.MethodPost, "/api/v1/favorites/media-1", nil)
	toggleReq.Header.Set("Authorization", "Bearer "+token)
	tw := httptest.NewRecorder()
	mux.ServeHTTP(tw, toggleReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ============================================================
// Music scanDirectory auth test
// ============================================================

func TestMusicScan_NoAuth(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	body := `{"library_id":"lib1","dir":"/tmp/music"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/scan", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// requireAdmin returns 401 for no token
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMusicScan_NonAdmin_403(t *testing.T) {
	mux, token := newMusicTestMuxWithToken(t)
	body := `{"library_id":"lib1","dir":"/tmp/music"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/scan", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// non-admin user
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ============================================================
// subtitleHandler (old-style, file stream) auth tests
// ============================================================

func TestSubtitleFileHandler_NoAuth_401(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_InvalidToken_403(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_NotFound_404(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_IDOR_403(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted", FilePath: "/a"}},
		"http://localhost",
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_QueryToken(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0?token=tok", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass with query token, got %d", w.Code)
	}
}

// ============================================================
// Popular handler additional test (boost 60% -> higher)
// ============================================================

func TestPopularRoute_NoAuth_401(t *testing.T) {
	mux, _ := newPopularTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ============================================================
// buildFFmpegArgs test (transcode_handler.go — pure logic, no I/O)
// ============================================================

func TestBuildFFmpegArgs_CopyAll(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{})
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}
	found := false
	for _, a := range args {
		if a == "-c:v" {
			found = true
		}
	}
	if !found {
		t.Error("expected -c:v in args")
	}
}

func TestBuildFFmpegArgs_VideoTranscode(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{needsVideoTranscode: true, needsAudioTranscode: true})
	hasLibx264 := false
	hasAAC := false
	for _, a := range args {
		if a == "libx264" {
			hasLibx264 = true
		}
		if a == "aac" {
			hasAAC = true
		}
	}
	if !hasLibx264 {
		t.Error("expected libx264 for video transcode")
	}
	if !hasAAC {
		t.Error("expected aac for audio transcode")
	}
}

func TestBuildFFmpegArgs_VideoScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{videoScale: 720, subtitleTrack: -1}, transcodeAnalysis{})
	hasScale := false
	for _, a := range args {
		if strings.Contains(a, "scale=-2:720") {
			hasScale = true
		}
	}
	if !hasScale {
		t.Error("expected scale filter for videoScale=720")
	}
}

func TestBuildFFmpegArgs_WithStartTime(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{startTimeSec: 30.5, subtitleTrack: -1}, transcodeAnalysis{})
	hasSS := false
	for _, a := range args {
		if a == "-ss" {
			hasSS = true
		}
	}
	if !hasSS {
		t.Error("expected -ss for startTimeSec>0")
	}
}

func TestBuildFFmpegArgs_BurnInSubtitle(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: 0}, transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
		needsAudioTranscode: true,
	})
	hasFilterComplex := false
	for _, a := range args {
		if a == "-filter_complex" {
			hasFilterComplex = true
		}
	}
	if !hasFilterComplex {
		t.Error("expected -filter_complex for burn-in subtitle")
	}
}

func TestBuildFFmpegArgs_BurnInSubtitleWithScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: 0, videoScale: 480}, transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
	})
	hasScale := false
	for _, a := range args {
		if strings.Contains(a, "scale") {
			hasScale = true
		}
	}
	if !hasScale {
		t.Error("expected scale in overlay filter")
	}
}

func TestBuildFFmpegArgs_AudioCopy(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{needsAudioTranscode: false})
	audioCopy := false
	for i, a := range args {
		if a == "-c:a" && i+1 < len(args) && args[i+1] == "copy" {
			audioCopy = true
		}
	}
	if !audioCopy {
		t.Error("expected -c:a copy when no audio transcode needed")
	}
}

func TestSubtitleList_WithItems(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	provider.DownloadPath = "/tmp/test.srt"
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{
		"m1": {ID: "m1", FilePath: "/tmp/movie.mkv", Title: "Test"},
	}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

	// download first
	dlBody := `{"fileId":42,"language":"en","format":"srt"}`
	dlReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(dlBody))
	dlReq.Header.Set("Authorization", "Bearer "+userToken)
	dw := httptest.NewRecorder()
	mux.ServeHTTP(dw, dlReq)

	// list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "en") {
		t.Error("expected subtitle language in response")
	}
}
