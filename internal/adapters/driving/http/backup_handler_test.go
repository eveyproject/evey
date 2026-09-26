package httpapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appbackup "github.com/evey-project/evey/internal/app/backup"
	backupmodel "github.com/evey-project/evey/internal/domain/backup"
	backupfakes "github.com/evey-project/evey/internal/domain/backup/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
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
	writeRL := ratelimit.New(100)

	mux := http.NewServeMux()
	registerBackupRoutes(mux, svc, gen, nil, rl, writeRL)
	return mux, token
}

// newBackupTestMuxWithStore is like newBackupTestMux but also returns the store
// so callers can inject errors or seed state to exercise specific paths.
func newBackupTestMuxWithStore(t *testing.T, isAdmin bool) (*http.ServeMux, string, *backupfakes.FakeBackupStore) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: isAdmin})

	store := backupfakes.NewFakeBackupStore()
	dumper := backupfakes.NewFakeDatabaseDumper()
	svc := appbackup.NewService(store, dumper, "1.0.0")
	rl := ratelimit.New(100)
	writeRL := ratelimit.New(100)

	mux := http.NewServeMux()
	registerBackupRoutes(mux, svc, gen, nil, rl, writeRL)
	return mux, token, store
}

// newBackupTestMuxWithDumper is like newBackupTestMux but also returns the dumper
// so callers can inject errors to exercise the internal-failure path.
func newBackupTestMuxWithDumper(t *testing.T, isAdmin bool) (*http.ServeMux, string, *backupfakes.FakeDatabaseDumper) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: isAdmin})

	store := backupfakes.NewFakeBackupStore()
	dumper := backupfakes.NewFakeDatabaseDumper()
	svc := appbackup.NewService(store, dumper, "1.0.0")
	rl := ratelimit.New(100)
	writeRL := ratelimit.New(100)

	mux := http.NewServeMux()
	registerBackupRoutes(mux, svc, gen, nil, rl, writeRL)
	return mux, token, dumper
}

// buildRestoreFormRequest constructs a multipart/form-data POST body with the
// given bytes as the "backup" file field.
func buildRestoreFormRequest(t *testing.T, archiveBytes []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("backup", "backup.tar.gz")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(archiveBytes); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// buildValidArchive builds a well-formed tar.gz backup with a matching sha256.
func buildValidArchive(t *testing.T) []byte {
	t.Helper()
	dump := []byte("-- fake dump\n")
	hash := sha256.Sum256(dump)
	m := &backupmodel.BackupManifest{
		ID:         "bk-test",
		CreatedAt:  time.Now().UTC(),
		Version:    "1.0.0",
		Tables:     appbackup.BackupTables,
		DumpSHA256: hex.EncodeToString(hash[:]),
	}
	manifestBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, name := range []struct {
		n string
		d []byte
	}{
		{"manifest.json", manifestBytes},
		{"dump.sql", dump},
	} {
		hdr := &tar.Header{Name: name.n, Size: int64(len(name.d)), Mode: 0640}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(name.d); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestListBackups_NoAuth_403(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
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
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRestoreBackup_InvalidArchive_422(t *testing.T) {
	// Sending a non-gzip payload must be classified as client error → 422.
	mux, token := newBackupTestMux(t, true)
	req := buildRestoreFormRequest(t, []byte("this is not a gzip archive"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid archive, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestRestoreBackup_InternalDumperError_500(t *testing.T) {
	// A valid archive that passes all integrity checks but whose postgres restore
	// fails must produce a 500 (internal error, not client fault).
	mux, token, dumper := newBackupTestMuxWithDumper(t, true)
	dumper.Err = errors.New("pg: connection refused")

	archive := buildValidArchive(t)
	req := buildRestoreFormRequest(t, archive)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for internal dumper error, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestRestoreBackup_ValidArchive_200(t *testing.T) {
	// A well-formed archive with a matching sha256 must be accepted → 200.
	mux, token := newBackupTestMux(t, true)
	archive := buildValidArchive(t)
	req := buildRestoreFormRequest(t, archive)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid archive, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestDeleteBackup_ErrNotFoundSentinel_404(t *testing.T) {
	// ErrNotFound from the store must map to 404 (not 500).
	// This tests the sentinel dispatch in the handler.
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent backup, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestDeleteBackup_StoreError_500(t *testing.T) {
	// A store failure that is NOT a not-found (e.g. disk full) must produce 500,
	// not 404 - otherwise the admin can't distinguish a missing backup from a real
	// failure.
	mux, token, store := newBackupTestMuxWithStore(t, true)
	store.Err = errors.New("disk full")
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/any-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for store error, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestDeleteBackup_Success_204(t *testing.T) {
	// DELETE on an existing backup must return 204 No Content.
	mux, token, store := newBackupTestMuxWithStore(t, true)
	// Seed a backup directly into the fake store so we have a known ID.
	m := &backupmodel.BackupManifest{
		ID:      "bk-del-test",
		Version: "1.0.0",
		Tables:  appbackup.BackupTables,
	}
	if err := store.Create(t.Context(), m, bytes.NewReader([]byte("dump"))); err != nil {
		t.Fatalf("seed backup: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/bk-del-test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for successful delete, got %d (body: %s)", w.Code, w.Body.String())
	}
}
