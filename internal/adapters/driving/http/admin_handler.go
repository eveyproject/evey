package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
	"github.com/gravity-zero/evey/internal/pkg/runtimedetect"
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
	claims, err := extractAdminClaims(r, h.tokenGen)
	if err != nil {
		return
	}
	ip := r.RemoteAddr
	ua := r.UserAgent()
	e := &activity.AuditLogEntry{
		UserID:       claims.UserID,
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

func (h *adminHandler) requireAdmin(r *http.Request) bool {
	_, err := extractAdminClaims(r, h.tokenGen)
	return err == nil
}

func (h *adminHandler) requireAdminWithResponse(w http.ResponseWriter, r *http.Request) bool {
	_, err := extractAdminClaims(r, h.tokenGen)
	if err == nil {
		return true
	}
	if errors.Is(err, errUnauthenticated) {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
	} else {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
	}
	return false
}

func (h *adminHandler) adminCheckWithQueryToken(r *http.Request) bool {
	_, err := extractAdminClaims(r, h.tokenGen)
	return err == nil
}

// --- register functions ---

func registerAdminRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen *auth.JWTGenerator, heavyRL, writeRL *ratelimit.Limiter, serverCtx context.Context, actCmdSvc *appactivity.CommandService) {
	h := &adminHandler{tokenGen: tokenGen, metaCmd: metaCmd, metaQuery: metaQuery, heavyRL: heavyRL, serverCtx: serverCtx, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("/api/v1/admin/media/", h.handleMedia)
	mux.HandleFunc("POST /api/v1/admin/metadata/refresh-all", ratelimit.MiddlewareFunc(writeRL, h.handleRefreshAll))
}

func registerLogsRoute(mux *http.ServeMux, logDir string, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, logDir: logDir}
	mux.HandleFunc("GET /api/v1/admin/logs", h.handleGetLogs)
	mux.HandleFunc("GET /api/v1/admin/logs/stream", h.handleStreamLogs)
}

func registerTranscodeProfileRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator, actCmdSvc *appactivity.CommandService, writeRL *ratelimit.Limiter) {
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("POST /api/v1/admin/transcoding/profiles", ratelimit.MiddlewareFunc(writeRL, h.handleCreateTranscodeProfile))
	mux.HandleFunc("PUT /api/v1/admin/transcoding/profiles/{id}", ratelimit.MiddlewareFunc(writeRL, h.handleUpdateTranscodeProfile))
	mux.HandleFunc("DELETE /api/v1/admin/transcoding/profiles/{id}", ratelimit.MiddlewareFunc(writeRL, h.handleDeleteTranscodeProfile))
}

func registerTranscodeCacheRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator, mediaGetter MediaItemGetter, actCmdSvc *appactivity.CommandService, writeRL *ratelimit.Limiter) {
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc, mediaGetter: mediaGetter, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("GET /api/v1/admin/transcode-cache", h.handleGetTranscodeCache)
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache", ratelimit.MiddlewareFunc(writeRL, h.handleClearTranscodeCache))
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache/{filename}", ratelimit.MiddlewareFunc(writeRL, h.handleDeleteTranscodeCacheFile))
}

func registerAvailableRuntimesRoute(mux *http.ServeMux, tokenGen *auth.JWTGenerator) {
	mux.HandleFunc("GET /api/v1/admin/plugin-runtimes/available", requireAdmin(tokenGen, func(w http.ResponseWriter, _ *http.Request) {
		runtimes := runtimedetect.DetectRuntimes()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"runtimes": runtimes}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))
}
