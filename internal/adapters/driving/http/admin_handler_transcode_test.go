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

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/transcoding"
	tcfakes "github.com/evey-project/evey/internal/domain/transcoding/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

type transcodeTestFixture struct {
	mux      *http.ServeMux
	adminTok string
	userTok  string
	cache    *tcfakes.FakeCacheStore
	profiles *tcfakes.FakeTranscodeProfileRepository
}

func newAdminTranscodeMux(t *testing.T) *transcodeTestFixture {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})

	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobs := tcfakes.NewFakeTranscodeJobRepository()
	profiles := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cache := tcfakes.NewFakeCacheStore()
	svc := apptranscoding.NewService(tc, hw, jobs, profiles, bus, 2, apptranscoding.WithCacheStore(cache))

	writeRL := ratelimit.New(1000)
	t.Cleanup(func() { writeRL.Stop() })

	mux := http.NewServeMux()
	registerTranscodeProfileRoutes(mux, svc, gen, nil, writeRL)
	registerTranscodeCacheRoutes(mux, svc, gen, nil, nil, writeRL)
	return &transcodeTestFixture{mux: mux, adminTok: adminTok, userTok: userTok, cache: cache, profiles: profiles}
}

// --- POST /api/v1/admin/transcoding/profiles ---

func TestAdminTranscode_CreateProfile_NoAuth_401(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminTranscode_CreateProfile_NonAdmin_403(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminTranscode_CreateProfile_Admin_201(t *testing.T) {
	f := newAdminTranscodeMux(t)
	body := `{"name":"1080p","videoCodec":"h264","audioCodec":"aac","container":"mp4","maxWidth":1920,"maxHeight":1080,"videoBitrate":5000000,"audioBitrate":128000,"preset":"medium"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	var out transcoding.TranscodeProfile
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID == "" || out.Name != "1080p" {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestAdminTranscode_CreateProfile_BadJSON_400(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- PUT/DELETE /api/v1/admin/transcoding/profiles/{id} ---

func TestAdminTranscode_UpdateProfile_NonAdmin_403(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/transcoding/profiles/tp-x", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminTranscode_UpdateProfile_Admin_OK(t *testing.T) {
	f := newAdminTranscodeMux(t)
	// seed an existing profile
	_ = f.profiles.Create(context.Background(), &transcoding.TranscodeProfile{ID: "tp-1", Name: "old", VideoCodec: "h264", Container: "mp4"})
	body := `{"name":"new","videoCodec":"h264","audioCodec":"aac","container":"mp4"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/transcoding/profiles/tp-1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminTranscode_DeleteProfile_Admin_204(t *testing.T) {
	f := newAdminTranscodeMux(t)
	_ = f.profiles.Create(context.Background(), &transcoding.TranscodeProfile{ID: "tp-2", Name: "n", VideoCodec: "h264", Container: "mp4"})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcoding/profiles/tp-2", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

// --- GET /api/v1/admin/transcode-cache ---

func TestAdminTranscode_GetCache_NoAuth_401(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode-cache", nil)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminTranscode_GetCache_NonAdmin_403(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminTranscode_GetCache_Admin_OK(t *testing.T) {
	f := newAdminTranscodeMux(t)
	f.cache.AddFile(transcoding.CacheFileInfo{Name: "a.mp4", Size: 1024, LastModified: time.Now()})
	f.cache.AddFile(transcoding.CacheFileInfo{Name: "b.mp4", Size: 2048, LastModified: time.Now()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		TotalSize int64 `json:"totalSize"`
		FileCount int   `json:"fileCount"`
		Files     []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.FileCount != 2 || out.TotalSize != 3072 {
		t.Errorf("unexpected: %+v", out)
	}
}

// --- DELETE /api/v1/admin/transcode-cache ---

func TestAdminTranscode_ClearCache_NonAdmin_403(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminTranscode_ClearCache_Admin_204(t *testing.T) {
	f := newAdminTranscodeMux(t)
	f.cache.AddFile(transcoding.CacheFileInfo{Name: "a.mp4", Size: 1, LastModified: time.Now()})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

// --- DELETE /api/v1/admin/transcode-cache/{filename} ---

func TestAdminTranscode_DeleteCacheFile_NoAuth_401(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache/a.mp4", nil)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminTranscode_DeleteCacheFile_NonAdmin_403(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache/a.mp4", nil)
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminTranscode_DeleteCacheFile_Admin_204(t *testing.T) {
	f := newAdminTranscodeMux(t)
	f.cache.AddFile(transcoding.CacheFileInfo{Name: "a.mp4", Size: 1, LastModified: time.Now()})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache/a.mp4", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestAdminTranscode_DeleteCacheFile_NotFound_404(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache/missing.mp4", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- UpdateProfile + DeleteProfile additional auth/validation coverage ---

func TestAdminTranscode_UpdateProfile_NoAuth_401(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/transcoding/profiles/tp-x", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminTranscode_UpdateProfile_BadJSON_400(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/transcoding/profiles/tp-1", strings.NewReader("garbage"))
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAdminTranscode_DeleteProfile_NoAuth_401(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcoding/profiles/tp-x", nil)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminTranscode_DeleteProfile_NonAdmin_403(t *testing.T) {
	f := newAdminTranscodeMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcoding/profiles/tp-x", nil)
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- DeleteCacheFile invalid filename branch ---

// mapMediaItemGetter is a trivial MediaItemGetter used to verify the display-name
// resolution path in handleGetTranscodeCache.
type mapMediaItemGetter struct {
	byID map[string]string // id → title
}

func (g *mapMediaItemGetter) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	title, ok := g.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &domainlibrary.MediaItem{ID: shared.MediaItemID(id), Title: title}, nil
}

// GetCache overrides the display name from mediaGetter when item exists.
func TestAdminTranscode_GetCache_ResolvesTitleFromMediaGetter(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobs := tcfakes.NewFakeTranscodeJobRepository()
	profiles := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cache := tcfakes.NewFakeCacheStore()
	cache.AddFile(transcoding.CacheFileInfo{Name: "mi-42", Size: 10, LastModified: time.Now()})
	svc := apptranscoding.NewService(tc, hw, jobs, profiles, bus, 2, apptranscoding.WithCacheStore(cache))

	writeRL := ratelimit.New(1000)
	t.Cleanup(func() { writeRL.Stop() })

	mux := http.NewServeMux()
	mediaGetter := &mapMediaItemGetter{byID: map[string]string{"mi-42": "My Movie"}}
	registerTranscodeCacheRoutes(mux, svc, gen, mediaGetter, nil, writeRL)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"name":"My Movie"`) {
		t.Errorf("expected title resolution, got %s", w.Body.String())
	}
}
