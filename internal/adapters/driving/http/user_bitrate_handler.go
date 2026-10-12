package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// userBitrateSetter persists a user's per-account max streaming bitrate
// (kbps). Satisfied by *appuser.BitrateLimitSetter, a thin repo-backed
// wrapper - no proto RPC exists for this admin-only cap (unlike the
// per-profile one UpdateProfile already exposes).
type userBitrateSetter interface {
	SetUserBitrateLimit(ctx context.Context, userID string, maxBitrateKbps int) error
}

type userBitrateHandler struct {
	getter UserBitrateGetter
	setter userBitrateSetter
}

// registerUserBitrateRoutes wires the admin "max bitrate per user" modal both
// frontends already call (web BitrateModal.tsx + mobile BitrateModal.tsx):
// neither the manual mux nor the proto ever exposed GET/PATCH
// .../bitrate-limit, so the modal has 404'd since it shipped. The read/write
// logic (QueryService.GetUserBitrateLimit / a repo-backed BitrateLimitSetter)
// already existed - only the wiring was missing.
func registerUserBitrateRoutes(mux *http.ServeMux, getter UserBitrateGetter, setter userBitrateSetter, tokenGen domainuser.TokenGenerator, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	h := &userBitrateHandler{getter: getter, setter: setter}
	mux.HandleFunc("GET /api/v1/admin/users/{id}/bitrate-limit", admin(h.get))
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/bitrate-limit", ratelimit.MiddlewareFunc(writeRL, admin(h.patch)))
}

func (h *userBitrateHandler) get(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.admin.GetUserBitrateLimit")
	defer span.End()
	userID := r.PathValue("id")
	span.SetAttributes(attribute.String("user_id", userID))

	limit, err := h.getter.GetUserBitrateLimit(r.Context(), userID)
	if err != nil {
		span.RecordError(err)
		// GetUserBitrateLimit's only failure mode is the underlying GetByID
		// lookup (postgres and the fakes both surface shared.ErrNotFound for
		// a missing row); any error is treated as "user not found" rather
		// than leaking a 500 for what is, from the admin panel, a bad id.
		slog.WarnContext(ctx, "get user bitrate limit failed", slog.String("user_id", userID), slog.Any("err", err))
		http.Error(w, `{"message":"user not found"}`, http.StatusNotFound)
		return
	}
	jsonEncodeResponse(w, map[string]int{"max_bitrate_kbps": limit})
}

type patchBitrateLimitRequest struct {
	MaxBitrateKbps int `json:"max_bitrate_kbps"`
}

func (h *userBitrateHandler) patch(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.admin.SetUserBitrateLimit")
	defer span.End()
	userID := r.PathValue("id")
	span.SetAttributes(attribute.String("user_id", userID))

	var body patchBitrateLimitRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if body.MaxBitrateKbps < 0 {
		http.Error(w, `{"message":"max_bitrate_kbps must be >= 0"}`, http.StatusBadRequest)
		return
	}

	if err := h.setter.SetUserBitrateLimit(r.Context(), userID, body.MaxBitrateKbps); err != nil {
		span.RecordError(err)
		slog.WarnContext(ctx, "set user bitrate limit failed", slog.String("user_id", userID), slog.Any("err", err))
		http.Error(w, `{"message":"user not found"}`, http.StatusNotFound)
		return
	}
	jsonEncodeResponse(w, map[string]int{"max_bitrate_kbps": body.MaxBitrateKbps})
}
