package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// --- shared helpers ---

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
