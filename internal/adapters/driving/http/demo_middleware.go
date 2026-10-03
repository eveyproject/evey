package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/evey-project/evey/internal/pkg/demo"
	"github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// DemoLockMiddleware refuses, on a demo instance, every HTTP route the demo
// policy locks. It asks the mux which pattern the request resolves to WITHOUT
// serving it (mux.Handler), so the decision is made on the registered pattern
// - the same string the contractguard test classifies - and it reaches the
// routes whose auth wrapper is hidden inside a handler constructor, which a
// per-route wrapper would have to be threaded through 27 call sites to cover.
//
// The gateway routes ("/") pass through: their RPCs are judged by the gRPC
// DemoInterceptor on the loopback hop.
func DemoLockMiddleware(mux *http.ServeMux, translator *i18n.Translator, next http.Handler) http.Handler {
	return DemoLockMiddlewareAt(demo.Public, mux, translator, next)
}

// DemoLockMiddlewareAt is DemoLockMiddleware at a given demo level.
func DemoLockMiddlewareAt(level demo.Level, mux *http.ServeMux, translator *i18n.Translator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if !demo.RouteLockedAt(level, r.Method, pattern) {
			next.ServeHTTP(w, r)
			return
		}
		refuseDemoLocked(w, r, translator, pattern)
	})
}

// refuseDemoLocked writes the demo's one refusal (403, code + sentence).
func refuseDemoLocked(w http.ResponseWriter, r *http.Request, translator *i18n.Translator, pattern string) {
	metrics.AuthzDeniedTotal.WithLabelValues("http", "demo_locked").Inc()
	slog.InfoContext(r.Context(), "demo: route locked",
		slog.String("method", r.Method), slog.String("pattern", pattern))
	// `code` carries the key so a client that knows the catalogue renders
	// its own sentence; `message` carries the sentence in the caller's
	// language so a client that only prints err.message never shows a
	// bare key or a bare 403.
	message := demo.ErrorKey
	if translator != nil {
		message = translator.T(i18n.ParseLocale(r.Header.Get("Accept-Language")), demo.ErrorKey)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": demo.ErrorKey, "message": message})
}
