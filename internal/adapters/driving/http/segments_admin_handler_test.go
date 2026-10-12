package httpapi

import (
	"bytes"
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

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// --- fakes ---

type fakeIntroDetectionTrigger struct {
	detectErr    error
	setErr       error
	deleteErr    error
	propagateIDs []string
	propagateErr error

	setCalled       bool
	deleteCalled    bool
	propagateCalled bool
	detectCalled    bool
}

func (f *fakeIntroDetectionTrigger) DetectIntro(_ context.Context, _ string) error {
	f.detectCalled = true
	return f.detectErr
}

func (f *fakeIntroDetectionTrigger) DetectSeason(_ context.Context, _ string) error { return nil }
func (f *fakeIntroDetectionTrigger) DetectSeries(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (f *fakeIntroDetectionTrigger) DetectIntroDryRun(_ context.Context, _ string) ([]domainlibrary.MediaSegment, error) {
	return nil, nil
}

func (f *fakeIntroDetectionTrigger) BackfillLibrary(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (f *fakeIntroDetectionTrigger) SetManualSegment(_ context.Context, _, _ string, _, _ int64) error {
	f.setCalled = true
	return f.setErr
}

func (f *fakeIntroDetectionTrigger) DeleteSegment(_ context.Context, _, _ string) error {
	f.deleteCalled = true
	return f.deleteErr
}

func (f *fakeIntroDetectionTrigger) PropagateSegment(_ context.Context, _, _ string) ([]string, error) {
	f.propagateCalled = true
	return f.propagateIDs, f.propagateErr
}

// --- shared helpers for these admin tests (real JWT tokens) ---

func segAdminTokens(t *testing.T) (TokenVerifier, string, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	admin, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true, ProfileID: "p-adm"})
	user, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false, ProfileID: "p-usr"})
	return gen, admin, user
}

// --- setSegmentHandler ---

func TestSetSegmentHandler_NoAuth_401(t *testing.T) {
	gen, _, _ := segAdminTokens(t)
	h := setSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/m1/segments", bytes.NewReader([]byte(`{"type":"intro","startMs":0,"endMs":60000}`)))
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSetSegmentHandler_NonAdmin_403(t *testing.T) {
	gen, _, userTok := segAdminTokens(t)
	h := setSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/m1/segments", bytes.NewReader([]byte(`{"type":"intro","startMs":0,"endMs":60000}`)))
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestSetSegmentHandler_MissingMediaID_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	h := setSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media//segments", bytes.NewReader([]byte(`{"type":"intro","startMs":0,"endMs":60000}`)))
	// no SetPathValue
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing media id, got %d", w.Code)
	}
}

func TestSetSegmentHandler_MediaNotFound_404(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	h := setSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{err: errors.New("not found")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/m1/segments", bytes.NewReader([]byte(`{"type":"intro","startMs":0,"endMs":60000}`)))
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSetSegmentHandler_BadJSON_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	h := setSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/m1/segments", bytes.NewReader([]byte("not json")))
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}
}

func TestSetSegmentHandler_ServiceError_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	svc := &fakeIntroDetectionTrigger{setErr: errors.New("type must be intro|outro|recap")}
	h := setSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/m1/segments", bytes.NewReader([]byte(`{"type":"bogus","startMs":0,"endMs":60000}`)))
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 from service error, got %d", w.Code)
	}
	if !svc.setCalled {
		t.Error("expected SetManualSegment to be called")
	}
}

func TestSetSegmentHandler_OK(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	svc := &fakeIntroDetectionTrigger{}
	h := setSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/m1/segments", bytes.NewReader([]byte(`{"type":"intro","startMs":0,"endMs":60000}`)))
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !svc.setCalled {
		t.Error("expected SetManualSegment to be called")
	}
}

// --- deleteSegmentHandler ---

func TestDeleteSegmentHandler_NonAdmin_403(t *testing.T) {
	gen, _, userTok := segAdminTokens(t)
	h := deleteSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/media/m1/segments?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDeleteSegmentHandler_MissingType_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	h := deleteSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing type, got %d", w.Code)
	}
}

func TestDeleteSegmentHandler_NotFound_404(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	h := deleteSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{err: errors.New("nope")})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/media/m1/segments?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteSegmentHandler_OK(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	svc := &fakeIntroDetectionTrigger{}
	h := deleteSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/media/m1/segments?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !svc.deleteCalled {
		t.Error("expected DeleteSegment to be called")
	}
}

func TestDeleteSegmentHandler_ServiceError_500(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	svc := &fakeIntroDetectionTrigger{deleteErr: errors.New("db gone")}
	h := deleteSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/media/m1/segments?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- propagateSegmentHandler ---

func TestPropagateSegmentHandler_NonAdmin_403(t *testing.T) {
	gen, _, userTok := segAdminTokens(t)
	h := propagateSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/segments/propagate?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPropagateSegmentHandler_OK(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	svc := &fakeIntroDetectionTrigger{propagateIDs: []string{"sibling1", "sibling2", "sibling3"}}
	h := propagateSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/segments/propagate?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"count":3`) {
		t.Errorf("expected count=3 in body, got %s", body)
	}
	if !strings.Contains(body, "sibling1") {
		t.Errorf("expected propagatedTo to include sibling1, got %s", body)
	}
}

func TestPropagateSegmentHandler_MissingType_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	h := propagateSegmentHandler(gen, &fakeIntroDetectionTrigger{}, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/segments/propagate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPropagateSegmentHandler_ServiceError_500(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	svc := &fakeIntroDetectionTrigger{propagateErr: errors.New("boom")}
	h := propagateSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/segments/propagate?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// The service's refusals quote user data ("got \"movie\""): the body must
// stay valid JSON whatever the message contains, or the admin panel shows a
// parse failure instead of the reason.
func TestSegmentsAdminHandlers_ErrorBodiesAreValidJSON(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	reason := `propagate only works on episodes, got "movie"`
	svc := &fakeIntroDetectionTrigger{propagateErr: errors.New(reason), setErr: errors.New(`bad "type"`)}

	h := propagateSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/segments/propagate?type=intro", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("propagate: body is not JSON: %v (%s)", err, w.Body.String())
	}
	if body["message"] != reason {
		t.Errorf("propagate message = %q", body["message"])
	}

	h = setSegmentHandler(gen, svc, allowAll(), &fakeMediaItemGetter{item: testMediaItem("lib1")})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/m1/segments", strings.NewReader(`{"type":"intro","startMs":0,"endMs":1000}`))
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("set: %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("set: body is not JSON: %v (%s)", err, w.Body.String())
	}
	if body["message"] != `bad "type"` {
		t.Errorf("set message = %q", body["message"])
	}
}
