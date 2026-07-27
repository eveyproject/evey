package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// registerBackgroundTasksRoute mounts GET /api/v1/admin/background-tasks.
//
// Admin-only, pure read: a JSON snapshot of every instrumented background worker
// (running/idle, the item it's on, items processed this run) so an operator can
// see WHAT is keeping the host busy - why the CPU isn't near idle. Sourced from
// the in-process bgtasks registry, so it's always cheap and never 5xx.
func registerBackgroundTasksRoute(mux *http.ServeMux, tokenGen TokenVerifier) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/background-tasks", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.BackgroundTasks")
		defer span.End()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(bgtasks.Snapshot()); err != nil {
			slog.WarnContext(r.Context(), "background-tasks: encode failed", slog.Any("err", err))
		}
	}))
}
