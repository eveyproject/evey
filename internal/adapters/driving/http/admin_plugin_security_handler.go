package httpapi

import (
	"context"
	"encoding/json"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"log/slog"
	"net/http"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// pluginSecurityDashboardProvider is the minimal service interface the handler needs.
type pluginSecurityDashboardProvider interface {
	GetSecurityDashboard(ctx context.Context) ([]domainplugin.PluginSecuritySnapshot, error)
	SandboxStatus() appplugin.SandboxStatus
}

// registerPluginSecurityDashboardRoute mounts GET /api/v1/admin/plugins/security-dashboard.
// Admin-only; no mutation; no library_id/media_id → no checkLibraryAccess needed.
func registerPluginSecurityDashboardRoute(mux *http.ServeMux, tokenGen domainuser.TokenGenerator, svc pluginSecurityDashboardProvider) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/plugins/security-dashboard", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.adminpluginsec.GetSecurityDashboard")
		defer span.End()
		r = r.WithContext(ctx)
		snapshots, err := svc.GetSecurityDashboard(r.Context())
		if err != nil {
			claims := HTTPClaimsFromContext(r.Context())
			userID := ""
			if claims != nil {
				userID = claims.UserID
			}
			slog.ErrorContext(ctx, "plugin security dashboard", slog.String("user_id", userID), slog.Any("err", err))
			http.Error(w, `{"message":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		if snapshots == nil {
			snapshots = []domainplugin.PluginSecuritySnapshot{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"plugins": snapshots,
			// Host-level diagnosis: shown even with zero plugins installed, so
			// an admin learns about a degraded sandbox before installing one.
			"sandbox": svc.SandboxStatus(),
		}); err != nil {
			slog.WarnContext(ctx, "plugin security dashboard: json encode", slog.Any("err", err))
		}
	}))
}
