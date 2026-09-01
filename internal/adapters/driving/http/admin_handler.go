package httpapi

import (
	"context"
	"encoding/json"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"log/slog"
	"net/http"

	otelcodes "go.opentelemetry.io/otel/codes"

	appactivity "github.com/evey-project/evey/internal/app/activity"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/runtimedetect"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type adminHandler struct {
	tokenGen       domainuser.TokenGenerator
	metaCmd        *appmetadata.CommandService
	metaQuery      *appmetadata.QueryService
	heavyRL        *ratelimit.Limiter
	activityCmdSvc *appactivity.CommandService
	logDir         string
	serverCtx      context.Context
}

func (h *adminHandler) audit(r *http.Request, action, resourceType string, resourceID *string, details map[string]any) {
	if h.activityCmdSvc == nil {
		return
	}
	claims := HTTPClaimsFromContext(r.Context())
	if claims == nil {
		return
	}
	ip := r.RemoteAddr
	ua := r.UserAgent()
	e := &activity.AuditLogEntry{
		UserID:       shared.UserID(claims.UserID),
		ProfileID:    shared.ProfileID(claims.ProfileID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		ClientIP:     &ip,
		UserAgent:    &ua,
	}
	if err := h.activityCmdSvc.CreateAuditEntry(r.Context(), e); err != nil {
		slog.WarnContext(r.Context(), "audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}

// --- register functions ---

func registerAdminRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen domainuser.TokenGenerator, heavyRL, writeRL *ratelimit.Limiter, serverCtx context.Context, actCmdSvc *appactivity.CommandService) {
	admin := httpAdminMiddleware(tokenGen)
	//nolint:exhaustruct // adminHandler is shared by two route groups; this one serves the metadata routes and deliberately omits logDir (set by registerLogsRoute).
	h := &adminHandler{tokenGen: tokenGen, metaCmd: metaCmd, metaQuery: metaQuery, heavyRL: heavyRL, serverCtx: serverCtx, activityCmdSvc: actCmdSvc}
	// Explicit method+path routes (Go 1.22+ mux): without them, the default
	// catch-all falls through to the gRPC gateway and every call 404s. A
	// legacy prefix pattern doesn't help once a sibling {id}/xxx pattern is
	// registered - the prefix loses the specificity race.
	mux.HandleFunc("POST /api/v1/admin/media/{id}/refresh-metadata", admin(func(w http.ResponseWriter, r *http.Request) {
		h.handleRefreshMetadata(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/reset-metadata", admin(func(w http.ResponseWriter, r *http.Request) {
		h.handleResetMetadata(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /api/v1/admin/media/{id}/search-metadata", admin(h.handleSearchMetadata))
	mux.HandleFunc("POST /api/v1/admin/metadata/refresh-all", ratelimit.MiddlewareFunc(writeRL, admin(h.handleRefreshAll)))
}

func registerLogsRoute(mux *http.ServeMux, logDir string, tokenGen domainuser.TokenGenerator) {
	admin := httpAdminMiddleware(tokenGen)
	//nolint:exhaustruct // adminHandler is shared by two route groups; this one serves the logs routes and deliberately omits the metadata services (set by registerAdminRoutes).
	h := &adminHandler{tokenGen: tokenGen, logDir: logDir}
	mux.HandleFunc("GET /api/v1/admin/logs", admin(h.handleGetLogs))
	mux.HandleFunc("GET /api/v1/admin/logs/search", admin(h.handleSearchLogs))
	mux.HandleFunc("GET /api/v1/admin/logs/stream", admin(h.handleStreamLogs))
}

func registerAvailableRuntimesRoute(mux *http.ServeMux, tokenGen domainuser.TokenGenerator) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/plugin-runtimes/available", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.AvailableRuntimes")
		defer span.End()
		runtimes := runtimedetect.DetectRuntimes()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"runtimes": runtimes}); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "json encode failed")
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}))
}
