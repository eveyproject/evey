package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
)

type artworkAdminFixture struct {
	mux        *http.ServeMux
	adminTok   string
	userTok    string
	metaCmd    *appmetadata.CommandService
	artworkRep *metafakes.FakeArtworkRepository
}

func newArtworkAdminMux(t *testing.T) *artworkAdminFixture {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})

	artRepo := metafakes.NewFakeArtworkRepository()
	metaCmd := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artRepo,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)

	dataDir := t.TempDir()
	mux := http.NewServeMux()
	registerArtworkAdminRoutes(mux, metaCmd, gen, context.Background(), dataDir, nil)

	return &artworkAdminFixture{
		mux: mux, adminTok: adminTok, userTok: userTok,
		metaCmd: metaCmd, artworkRep: artRepo,
	}
}

// --- GET /api/v1/admin/artworks/stats ---

func TestArtworkAdmin_Stats_NoAuth_401(t *testing.T) {
	f := newArtworkAdminMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artworks/stats", nil)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestArtworkAdmin_Stats_NonAdmin_403(t *testing.T) {
	f := newArtworkAdminMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artworks/stats", nil)
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestArtworkAdmin_Stats_Admin_200(t *testing.T) {
	f := newArtworkAdminMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artworks/stats", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["totalRecords"]; !ok {
		t.Error("missing totalRecords field")
	}
	if _, ok := resp["missingLocalPath"]; !ok {
		t.Error("missing missingLocalPath field")
	}
}

// --- POST /api/v1/admin/artworks/download-all ---

func TestArtworkAdmin_DownloadAll_NoAuth_401(t *testing.T) {
	f := newArtworkAdminMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artworks/download-all", nil)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestArtworkAdmin_DownloadAll_NonAdmin_403(t *testing.T) {
	f := newArtworkAdminMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artworks/download-all", nil)
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestArtworkAdmin_DownloadAll_Admin_200(t *testing.T) {
	f := newArtworkAdminMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artworks/download-all", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["jobId"]; !ok {
		t.Error("missing jobId field")
	}
}

// --- GET /api/v1/admin/artworks/download-all/{jobId} ---

func TestArtworkAdmin_DownloadAllStatus_NotFound_404(t *testing.T) {
	f := newArtworkAdminMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artworks/download-all/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestArtworkAdmin_DownloadAll_Lifecycle(t *testing.T) {
	f := newArtworkAdminMux(t)

	// 1. Start a download job
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artworks/download-all", nil)
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start download-all: expected 200, got %d", w.Code)
	}
	var startResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&startResp); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	jobID, _ := startResp["jobId"].(string)
	if jobID == "" {
		t.Fatal("expected non-empty jobId")
	}

	// 2. Poll status
	// No records to download → job completes immediately or quickly
	deadline := time.Now().Add(5 * time.Second)
	var statusResp map[string]any
	for time.Now().Before(deadline) {
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artworks/download-all/"+jobID, nil)
		req2.Header.Set("Authorization", "Bearer "+f.adminTok)
		w2 := httptest.NewRecorder()
		f.mux.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("poll status: expected 200, got %d", w2.Code)
		}
		if err := json.NewDecoder(w2.Body).Decode(&statusResp); err != nil {
			t.Fatalf("decode status response: %v", err)
		}
		if statusResp["status"] == "done" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if statusResp["status"] != "done" {
		t.Errorf("expected status done, got %v", statusResp["status"])
	}
}
