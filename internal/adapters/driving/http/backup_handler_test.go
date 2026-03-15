package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appbackup "github.com/gravity-zero/evey/internal/app/backup"
	backupfakes "github.com/gravity-zero/evey/internal/domain/backup/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func newBackupTestMux(t *testing.T, isAdmin bool) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: isAdmin})

	store := backupfakes.NewFakeBackupStore()
	dumper := backupfakes.NewFakeDatabaseDumper()
	svc := appbackup.NewService(store, dumper, "1.0.0")
	rl := ratelimit.New(100)

	mux := http.NewServeMux()
	registerBackupRoutes(mux, svc, gen, rl)
	return mux, token
}

func TestBackupRequiresAdmin(t *testing.T) {
	tv := validUserValidator(false)
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/backups", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestListBackups_NoAuth_403(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListBackups_NonAdmin_403(t *testing.T) {
	mux, token := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListBackups_Admin_200(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCreateBackup_NoAuth_403(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
