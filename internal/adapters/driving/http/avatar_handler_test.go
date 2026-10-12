package httpapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// 1×1 PNG (valid magic bytes so http.DetectContentType returns image/png).
var pngPixel = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
	0x54, 0x08, 0x99, 0x63, 0x60, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC, 0x33, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

func seedProfile(t *testing.T, repo domainuser.ProfileRepository, id, userID string) *domainuser.Profile {
	t.Helper()
	p := &domainuser.Profile{ID: shared.ProfileID(id), UserID: shared.UserID(userID), Name: "P"}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func newUploadRequest(t *testing.T, profileID, fieldName string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(fieldName, "a.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(body)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/"+profileID+"/avatar", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("profileId", profileID)
	return req
}

func TestAvatarUpload_Success(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()
	p := seedProfile(t, repo, "p1", "u1")

	h := avatarUploadHandler(repo, tmp)
	req := newUploadRequest(t, string(p.ID), "avatar", pngPixel)
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", ProfileID: "p1"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "avatars/p1.png") {
		t.Errorf("expected avatarPath in body, got %s", w.Body.String())
	}
	// File written to disk
	if _, err := os.Stat(filepath.Join(tmp, "avatars", "p1.png")); err != nil {
		t.Errorf("avatar file not written: %v", err)
	}
	// Profile updated
	got, _ := repo.GetByID(context.Background(), "p1")
	if got.AvatarPath == nil || *got.AvatarPath != filepath.Join("avatars", "p1.png") {
		t.Errorf("avatar path on profile not updated: %+v", got.AvatarPath)
	}
}

func TestAvatarUpload_ForbiddenOtherUser(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()
	seedProfile(t, repo, "p1", "u-owner")

	h := avatarUploadHandler(repo, tmp)
	req := newUploadRequest(t, "p1", "avatar", pngPixel)
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u-intruder"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAvatarUpload_ProfileNotFound(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()

	h := avatarUploadHandler(repo, tmp)
	req := newUploadRequest(t, "ghost", "avatar", pngPixel)
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAvatarUpload_RejectsNonImage(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()
	seedProfile(t, repo, "p1", "u1")

	h := avatarUploadHandler(repo, tmp)
	req := newUploadRequest(t, "p1", "avatar", []byte("not-an-image plain text body"))
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAvatarUpload_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()
	seedProfile(t, repo, "p1", "u1")

	h := avatarUploadHandler(repo, tmp)
	// multipart body without "avatar" field
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_, _ = mw.CreateFormFile("other", "x.png")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/p1/avatar", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("profileId", "p1")
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAvatarServe_Success(t *testing.T) {
	tmp := t.TempDir()
	avatarsDir := filepath.Join(tmp, "avatars")
	_ = os.MkdirAll(avatarsDir, 0o755)
	_ = os.WriteFile(filepath.Join(avatarsDir, "p1.png"), pngPixel, 0o644)

	repo := userfakes.NewFakeProfileRepository()
	ap := filepath.Join("avatars", "p1.png")
	p := &domainuser.Profile{ID: "p1", UserID: "u1", AvatarPath: &ap}
	_ = repo.Create(context.Background(), p)

	h := avatarServeHandler(repo, tmp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/p1/avatar", nil)
	req.SetPathValue("profileId", "p1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Cache-Control") == "" {
		t.Error("expected Cache-Control header")
	}
	if w.Body.Len() == 0 {
		t.Error("expected body")
	}
}

func TestAvatarServe_NotFound_NoAvatar(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()
	_ = repo.Create(context.Background(), &domainuser.Profile{ID: "p1", UserID: "u1"})

	h := avatarServeHandler(repo, tmp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/p1/avatar", nil)
	req.SetPathValue("profileId", "p1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAvatarServe_ProfileNotFound(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()

	h := avatarServeHandler(repo, tmp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/ghost/avatar", nil)
	req.SetPathValue("profileId", "ghost")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAvatarServe_PathTraversalBlocked(t *testing.T) {
	tmp := t.TempDir()
	repo := userfakes.NewFakeProfileRepository()
	// Craft avatarPath escaping dataDir.
	evil := "../../../etc/passwd"
	_ = repo.Create(context.Background(), &domainuser.Profile{ID: "p1", UserID: "u1", AvatarPath: &evil})

	h := avatarServeHandler(repo, tmp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/p1/avatar", nil)
	req.SetPathValue("profileId", "p1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

// The serve handler contains the only guard that stops a stored avatar_path
// from escaping the data directory. It rests on two filepath.Abs calls whose
// errors were discarded: Abs returns "" on failure, so a failure of the dataDir
// call alone would turn the containment test into "is this path absolute?" and
// let the traversal through. The guard must refuse a path it cannot verify, and
// refuse a traversal it can.
func TestAvatarServe_RefusesPathOutsideDataDir(t *testing.T) {
	repo := userfakes.NewFakeProfileRepository()
	p := seedProfile(t, repo, "p-trav", "u1")

	escape := filepath.Join("..", "..", "etc", "passwd")
	p.AvatarPath = &escape
	if err := repo.Update(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	dataDir := t.TempDir()
	h := avatarServeHandler(repo, dataDir)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/p-trav/avatar", nil)
	req.SetPathValue("profileId", "p-trav")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("a stored avatar path pointing outside the data directory returned %d, "+
			"want 403: the containment guard is the only thing standing between a bad "+
			"avatar_path and an arbitrary file read", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "root:") {
		t.Fatal("the response carried the contents of a file outside the data directory")
	}
}
