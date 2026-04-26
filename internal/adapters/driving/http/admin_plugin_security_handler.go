package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// pluginSecurityDashboardProvider is the minimal service interface the handler needs.
type pluginSecurityDashboardProvider interface {
	GetSecurityDashboard(ctx context.Context) ([]domainplugin.PluginSecuritySnapshot, error)
}

// registerPluginSecurityDashboardRoute mounts GET /api/v1/admin/plugins/security-dashboard.
// Admin-only; no mutation; no library_id/media_id → no checkLibraryAccess needed.
func registerPluginSecurityDashboardRoute(mux *http.ServeMux, tokenGen *auth.JWTGenerator, svc pluginSecurityDashboardProvider) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/plugins/security-dashboard", admin(func(w http.ResponseWriter, r *http.Request) {
		snapshots, err := svc.GetSecurityDashboard(r.Context())
		if err != nil {
			slog.Error("plugin security dashboard", slog.Any("err", err))
			http.Error(w, `{"message":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		if snapshots == nil {
			snapshots = []domainplugin.PluginSecuritySnapshot{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"plugins": snapshots}); err != nil {
			slog.Warn("plugin security dashboard: json encode", slog.Any("err", err))
		}
	}))
}
