package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/eveyproject/evey/internal/app/updatecheck"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// UpdateChecker is what the update-status routes read: the last answer, and
// a check on demand.
type UpdateChecker interface {
	Status() updatecheck.Status
	CheckNow(ctx context.Context) updatecheck.Status
}

// registerUpdateStatusRoute mounts:
//
//	GET  /api/v1/admin/update-status        the last check's answer
//	POST /api/v1/admin/update-status/check  check now (at most once a minute)
//
// Admin-only. Nothing here updates anything: the dashboard tells the admin a
// newer image exists, the admin pulls it.
func registerUpdateStatusRoute(mux *http.ServeMux, tokenGen TokenVerifier, checker UpdateChecker) {
	if checker == nil {
		return
	}
	admin := httpAdminMiddleware(tokenGen)
	write := func(w http.ResponseWriter, r *http.Request, st updatecheck.Status) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(st); err != nil {
			slog.WarnContext(r.Context(), "update-status: encode failed", slog.Any("err", err))
		}
	}
	mux.HandleFunc("GET /api/v1/admin/update-status", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.UpdateStatus")
		defer span.End()
		write(w, r, checker.Status())
	}))
	mux.HandleFunc("POST /api/v1/admin/update-status/check", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.admin.UpdateStatus.Check")
		defer span.End()
		write(w, r, checker.CheckNow(ctx))
	}))
}
