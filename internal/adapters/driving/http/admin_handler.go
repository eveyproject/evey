package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	otelcodes "go.opentelemetry.io/otel/codes"

	appactivity "github.com/evey-project/evey/internal/app/activity"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/runtimedetect"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type adminHandler struct {
	tokenGen       *auth.JWTGenerator
	metaCmd        *appmetadata.CommandService
	metaQuery      *appmetadata.QueryService
	heavyRL        *ratelimit.Limiter
	activityCmdSvc *appactivity.CommandService
	logDir         string
	transcodeSvc   *apptranscoding.Service
	mediaGetter    MediaItemGetter
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
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		ClientIP:     &ip,
		UserAgent:    &ua,
	}
	if err := h.activityCmdSvc.CreateAuditEntry(r.Context(), e); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}

// --- register functions ---

func registerAdminRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen *auth.JWTGenerator, heavyRL, writeRL *ratelimit.Limiter, serverCtx context.Context, actCmdSvc *appactivity.CommandService) {
	admin := httpAdminMiddleware(tokenGen)
	h := &adminHandler{tokenGen: tokenGen, metaCmd: metaCmd, metaQuery: metaQuery, heavyRL: heavyRL, serverCtx: serverCtx, activityCmdSvc: actCmdSvc}
	// Explicit method+path routes (Go 1.22+ mux): without them, the default
	// catch-all falls through to the gRPC gateway and every call 404s. A
	// legacy prefix pattern doesn't help once a sibling {id}/xxx pattern is
	// registered — the prefix loses the specificity race.
	mux.HandleFunc("POST /api/v1/admin/media/{id}/refresh-metadata", admin(func(w http.ResponseWriter, r *http.Request) {
		h.handleRefreshMetadata(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/reset-metadata", admin(func(w http.ResponseWriter, r *http.Request) {
		h.handleResetMetadata(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /api/v1/admin/media/{id}/search-metadata", admin(h.handleSearchMetadata))
	mux.HandleFunc("POST /api/v1/admin/metadata/refresh-all", ratelimit.MiddlewareFunc(writeRL, admin(h.handleRefreshAll)))
}

func registerLogsRoute(mux *http.ServeMux, logDir string, tokenGen *auth.JWTGenerator) {
	admin := httpAdminMiddleware(tokenGen)
	h := &adminHandler{tokenGen: tokenGen, logDir: logDir}
	mux.HandleFunc("GET /api/v1/admin/logs", admin(h.handleGetLogs))
	mux.HandleFunc("GET /api/v1/admin/logs/stream", admin(h.handleStreamLogs))
}

func registerTranscodeProfileRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator, actCmdSvc *appactivity.CommandService, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("POST /api/v1/admin/transcoding/profiles", ratelimit.MiddlewareFunc(writeRL, admin(h.handleCreateTranscodeProfile)))
	mux.HandleFunc("PUT /api/v1/admin/transcoding/profiles/{id}", ratelimit.MiddlewareFunc(writeRL, admin(h.handleUpdateTranscodeProfile)))
	mux.HandleFunc("DELETE /api/v1/admin/transcoding/profiles/{id}", ratelimit.MiddlewareFunc(writeRL, admin(h.handleDeleteTranscodeProfile)))
}

func registerTranscodeCacheRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator, mediaGetter MediaItemGetter, actCmdSvc *appactivity.CommandService, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc, mediaGetter: mediaGetter, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("GET /api/v1/admin/transcode-cache", admin(h.handleGetTranscodeCache))
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache", ratelimit.MiddlewareFunc(writeRL, admin(h.handleClearTranscodeCache)))
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache/{filename}", ratelimit.MiddlewareFunc(writeRL, admin(h.handleDeleteTranscodeCacheFile)))
}

func registerAvailableRuntimesRoute(mux *http.ServeMux, tokenGen *auth.JWTGenerator) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/plugin-runtimes/available", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.AvailableRuntimes")
		defer span.End()
		runtimes := runtimedetect.DetectRuntimes()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"runtimes": runtimes}); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "json encode failed")
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))
}
