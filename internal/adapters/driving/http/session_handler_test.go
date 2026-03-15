package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainplayback "github.com/gravity-zero/evey/internal/domain/playback"
)

type fakeActiveSessionsSvc struct {
	sessions []*domainplayback.ActiveSessionInfo
	err      error
}

func (f *fakeActiveSessionsSvc) GetActiveSessions(_ context.Context) ([]*domainplayback.ActiveSessionInfo, error) {
	return f.sessions, f.err
}

type fakeStopStreamSvc struct {
	err error
}

func (f *fakeStopStreamSvc) StopStream(_ context.Context, _ string) error {
	return f.err
}

func TestActiveSessionsHandler_Forbidden_NonAdmin(t *testing.T) {
	h := activeSessionsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeActiveSessionsSvc{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sessions", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestActiveSessionsHandler_Admin_OK(t *testing.T) {
	now := time.Now()
	h := activeSessionsHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeActiveSessionsSvc{sessions: []*domainplayback.ActiveSessionInfo{
			{
				SessionID:  "sess1",
				MediaTitle: "Test Movie",
				StartedAt:  now,
			},
		}},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sessions", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sess1") {
		t.Error("expected session id in response")
	}
}

func TestKillSessionHandler_Forbidden_NonAdmin(t *testing.T) {
	h := killSessionHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeStopStreamSvc{},
	)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/sessions/sess1", nil)
	req.SetPathValue("id", "sess1")
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestKillSessionHandler_Admin_OK(t *testing.T) {
	h := killSessionHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeStopStreamSvc{},
	)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/sessions/sess1", nil)
	req.SetPathValue("id", "sess1")
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestKillSessionHandler_Admin_NotFound(t *testing.T) {
	h := killSessionHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeStopStreamSvc{err: errTest},
	)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/sessions/sess1", nil)
	req.SetPathValue("id", "sess1")
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestActiveSessionsHandler_InvalidToken_Forbidden(t *testing.T) {
	h := activeSessionsHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeActiveSessionsSvc{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sessions", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestActiveSessionsHandler_ServiceError_InternalError(t *testing.T) {
	h := activeSessionsHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeActiveSessionsSvc{err: errTest},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sessions", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestKillSessionHandler_InvalidToken_Forbidden(t *testing.T) {
	h := killSessionHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeStopStreamSvc{},
	)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/sessions/sess1", nil)
	req.SetPathValue("id", "sess1")
	req.Header.Set("Authorization", "Bearer invalid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
