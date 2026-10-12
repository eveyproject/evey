package httpapi

import (
	"context"
	"encoding/json"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"log/slog"
	"net/http"
	"time"

	appqoe "github.com/eveyproject/evey/internal/app/qoe"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// qoeDashboardProvider is the minimal contract the QoE admin endpoint needs.
type qoeDashboardProvider interface {
	GetDashboard(ctx context.Context, window time.Duration) (*appqoe.Dashboard, error)
}

// registerQoEDashboardRoute mounts GET /api/v1/admin/qoe/dashboard?window=24h|7d.
//
// Admin-only; pure read. Returns {} when the QoE service isn't wired yet
// (dev without Phase 1 storage), never 5xx - the dashboard UI handles the
// empty state gracefully.
func registerQoEDashboardRoute(mux *http.ServeMux, tokenGen domainuser.TokenGenerator, svc qoeDashboardProvider) {
	if svc == nil {
		// No service wired (tests without Postgres). Don't register the route
		// so callers get a clean 404 rather than a confusing 500.
		return
	}
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/qoe/dashboard", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.adminqoe.GetDashboard")
		defer span.End()
		r = r.WithContext(ctx)

		window := parseQoEWindow(r.URL.Query().Get("window"))

		dash, err := svc.GetDashboard(r.Context(), window)
		if err != nil {
			claims := HTTPClaimsFromContext(r.Context())
			userID := ""
			if claims != nil {
				userID = claims.UserID
			}
			slog.ErrorContext(ctx, "qoe dashboard", slog.String("user_id", userID), slog.Any("err", err))
			http.Error(w, `{"message":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		if dash.TopStarved == nil {
			dash.TopStarved = []appqoe.StarvedTitle{}
		}
		if dash.ByDevice == nil {
			dash.ByDevice = []appqoe.DeviceBreakdown{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(dash); err != nil {
			slog.WarnContext(ctx, "qoe dashboard: json encode", slog.Any("err", err))
		}
	}))
}

// parseQoEWindow accepts "24h" / "7d" - defaults to 24h on anything else so a
// typo'd URL doesn't 400 the dashboard. Keep it minimal until we actually
// want sliders in the UI.
func parseQoEWindow(s string) time.Duration {
	switch s {
	case "7d", "7days", "week":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}
