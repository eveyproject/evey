package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	appconfig "github.com/eveyproject/evey/internal/app/config"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// registerConfigDefaultsRoute serves what the language-and-region settings
// resolve to when left empty, so the admin panel shows the real default
// ("Default: French") rather than a placeholder.
func registerConfigDefaultsRoute(mux *http.ServeMux, tokenGen TokenVerifier, repo shared.ServerConfigRepository, configLanguage string) {
	if repo == nil {
		return
	}
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/config/defaults", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.config.Defaults")
		defer span.End()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		out := appconfig.LocaleDefaults(ctx, repo, configLanguage)
		// The server's own clock: the panel shows the effective time from it,
		// since the server - not the admin's device - applies opening hours.
		out["now"] = time.Now().UTC().Format(time.RFC3339)
		if err := json.NewEncoder(w).Encode(out); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	}))
}
