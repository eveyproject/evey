package httpapi

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	transcodingfakes "github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newTranscodeProfileTestMux(t *testing.T) (*http.ServeMux, *auth.JWTGenerator, *transcodingfakes.FakeTranscodeProfileRepository) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tokenGen := auth.NewJWTGenerator(key, time.Minute)

	profileRepo := transcodingfakes.NewFakeTranscodeProfileRepository()
	jobRepo := transcodingfakes.NewFakeTranscodeJobRepository()
	transcoder := transcodingfakes.NewFakeTranscoder()
	hwDetector := transcodingfakes.NewFakeHWDetector()
	bus := sharedfakes.NewFakeEventBus()

	svc := apptranscoding.NewService(transcoder, hwDetector, jobRepo, profileRepo, bus, 0)

	mux := http.NewServeMux()
	registerTranscodeProfileRoutes(mux, svc, tokenGen, nil)
	return mux, tokenGen, profileRepo
}

func adminToken(t *testing.T, tokenGen *auth.JWTGenerator) string {
	t.Helper()
	tok, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func nonAdminToken(t *testing.T, tokenGen *auth.JWTGenerator) string {
	t.Helper()
	tok, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func validProfileBody() []byte {
	b, _ := json.Marshal(map[string]any{
		"name":         "1080p H264",
		"videoCodec":   "h264",
		"audioCodec":   "aac",
		"container":    "mp4",
		"maxWidth":     1920,
		"maxHeight":    1080,
		"videoBitrate": 4000000,
		"audioBitrate": 128000,
		"preset":       "fast",
	})
	return b
}

// POST /api/v1/admin/transcoding/profiles

func TestCreateTranscodeProfile_NonAdmin_Forbidden(t *testing.T) {
	mux, tokenGen, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", bytes.NewReader(validProfileBody()))
	req.Header.Set("Authorization", "Bearer "+nonAdminToken(t, tokenGen))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCreateTranscodeProfile_InvalidBody_BadRequest(t *testing.T) {
	mux, tokenGen, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateTranscodeProfile_Admin_Created(t *testing.T) {
	mux, tokenGen, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", bytes.NewReader(validProfileBody()))
	req.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

// PUT /api/v1/admin/transcoding/profiles/{id}

func TestUpdateTranscodeProfile_NonAdmin_Forbidden(t *testing.T) {
	mux, tokenGen, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/transcoding/profiles/tp-1", bytes.NewReader(validProfileBody()))
	req.Header.Set("Authorization", "Bearer "+nonAdminToken(t, tokenGen))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestUpdateTranscodeProfile_NonExistent_InternalError(t *testing.T) {
	mux, tokenGen, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/transcoding/profiles/tp-nonexistent", bytes.NewReader(validProfileBody()))
	req.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// fake repo returns ErrNotFound → service propagates it → handler returns 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for missing profile, got %d", w.Code)
	}
}

func TestUpdateTranscodeProfile_Admin_OK(t *testing.T) {
	mux, tokenGen, profileRepo := newTranscodeProfileTestMux(t)

	// seed a profile first via Create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", bytes.NewReader(validProfileBody()))
	createReq.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	createW := httptest.NewRecorder()
	mux.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("seed create failed: %d", createW.Code)
	}

	var created map[string]any
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	profileID, _ := created["ID"].(string)
	if profileID == "" {
		// check for lowercase key
		profileID, _ = created["id"].(string)
	}
	// if still empty, get from repo
	if profileID == "" {
		ctx := createReq.Context()
		_ = ctx
		profiles, _ := profileRepo.List(createReq.Context())
		if len(profiles) == 0 {
			t.Fatal("no profiles in repo after create")
		}
		profileID = profiles[0].ID
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/transcoding/profiles/"+profileID, bytes.NewReader(validProfileBody()))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	updateW := httptest.NewRecorder()
	mux.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", updateW.Code)
	}
}

// DELETE /api/v1/admin/transcoding/profiles/{id}

func TestDeleteTranscodeProfile_NonAdmin_Forbidden(t *testing.T) {
	mux, tokenGen, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcoding/profiles/tp-1", nil)
	req.Header.Set("Authorization", "Bearer "+nonAdminToken(t, tokenGen))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDeleteTranscodeProfile_NonExistent_InternalError(t *testing.T) {
	mux, tokenGen, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcoding/profiles/tp-nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// fake repo returns ErrNotFound → handler returns 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for missing profile, got %d", w.Code)
	}
}

func TestDeleteTranscodeProfile_Admin_NoContent(t *testing.T) {
	mux, tokenGen, profileRepo := newTranscodeProfileTestMux(t)

	// seed a profile
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", bytes.NewReader(validProfileBody()))
	createReq.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	createW := httptest.NewRecorder()
	mux.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("seed create failed: %d", createW.Code)
	}

	profiles, _ := profileRepo.List(createReq.Context())
	if len(profiles) == 0 {
		t.Fatal("no profiles after create")
	}
	profileID := profiles[0].ID

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcoding/profiles/"+profileID, nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken(t, tokenGen))
	delW := httptest.NewRecorder()
	mux.ServeHTTP(delW, delReq)
	if delW.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", delW.Code)
	}
}

func TestCreateTranscodeProfile_NoToken_Forbidden(t *testing.T) {
	mux, _, _ := newTranscodeProfileTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcoding/profiles", bytes.NewReader(validProfileBody()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
