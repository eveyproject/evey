package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evey-project/evey/internal/pkg/demo"
	"github.com/evey-project/evey/internal/pkg/i18n"
)

// The demo lock judges the PATTERN the mux resolves, not the raw path, so a
// locked route stays locked whatever id or query it is called with, and the
// gateway ("/") passes through to the gRPC interceptor. Both directions are
// pinned - a lock that also blocked playback would be a demo of nothing.
func TestDemoLockMiddleware_JudgesTheResolvedPattern(t *testing.T) {
	mux := http.NewServeMux()
	served := map[string]bool{}
	mark := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { served[name] = true; w.WriteHeader(http.StatusNoContent) }
	}
	mux.HandleFunc("GET /api/v1/admin/backups/{id}", mark("backup-download"))   // LockedRoutes
	mux.HandleFunc("GET /api/v1/admin/backups", mark("backup-list"))            // OpenRoutes
	mux.HandleFunc("POST /api/v1/media/{id}/stream", mark("stream"))            // OpenRoutes: the product
	mux.HandleFunc("POST /api/v1/admin/brand-new", mark("brand-new"))           // unclassified admin write: locked by default
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mark("artwork")) // plain read, never judged
	mux.HandleFunc("/", mark("gateway"))                                        // grpc-gateway + SPA

	tr, err := i18n.NewFromDir("../../../../i18n")
	if err != nil {
		t.Fatalf("load catalogue: %v", err)
	}
	h := DemoLockMiddleware(mux, tr, mux)
	call := func(method, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, target, nil)
		req.Header.Set("Accept-Language", "fr")
		h.ServeHTTP(w, req)
		return w
	}

	w := call(http.MethodGet, "/api/v1/admin/backups/42?token=abc")
	if w.Code != http.StatusForbidden {
		t.Fatalf("locked route: status %d, want 403", w.Code)
	}
	var body struct{ Code, Message string }
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if body.Code != demo.ErrorKey {
		t.Errorf("refusal code = %q, want the catalogue key so a client can render its own sentence", body.Code)
	}
	if body.Message != tr.T("fr", demo.ErrorKey) || body.Message == demo.ErrorKey {
		t.Errorf("refusal message = %q, want the French sentence, never a bare key or a bare 403", body.Message)
	}
	if served["backup-download"] {
		t.Error("the locked handler ran")
	}

	if w := call(http.MethodPost, "/api/v1/admin/brand-new"); w.Code != http.StatusForbidden {
		t.Errorf("unclassified admin write: status %d, want 403 (deny by default)", w.Code)
	}

	for name, req := range map[string][2]string{
		"backup-list": {http.MethodGet, "/api/v1/admin/backups"},
		"stream":      {http.MethodPost, "/api/v1/media/42/stream"},
		"artwork":     {http.MethodGet, "/api/v1/media/42/artwork/poster"},
		"gateway":     {http.MethodPost, "/evey.v1.AdminSystemService/RestartServer"},
	} {
		if w := call(req[0], req[1]); w.Code != http.StatusNoContent || !served[name] {
			t.Errorf("%s: status %d, served=%v - an open route must reach its handler", name, w.Code, served[name])
		}
	}
}
