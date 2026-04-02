package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	goruntime "runtime"
	"sync"
	"time"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appconfig "github.com/gravity-zero/evey/internal/app/config"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
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
	userSvc        *appuser.UserService
	userQuery      *appuser.QueryService
	activityCmdSvc *appactivity.CommandService
	logDir         string
	startTime      time.Time
	cfgSvc         *appconfig.Service
	transcodeSvc   *apptranscoding.Service
	mediaGetter    MediaItemGetter
	bioEnricher    appmetadata.PeopleBioEnricher
	serverCtx      context.Context

	statsMu      sync.Mutex
	cachedMS     goruntime.MemStats
	cacheExpires time.Time
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

func registerAdminRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen *auth.JWTGenerator, heavyRL *ratelimit.Limiter, serverCtx context.Context, actCmdSvc *appactivity.CommandService, bioEnricher appmetadata.PeopleBioEnricher) {
	h := &adminHandler{tokenGen: tokenGen, metaCmd: metaCmd, metaQuery: metaQuery, heavyRL: heavyRL, serverCtx: serverCtx, activityCmdSvc: actCmdSvc, bioEnricher: bioEnricher}
	mux.HandleFunc("/api/v1/admin/media/", h.handleMedia)
	mux.HandleFunc("POST /api/v1/admin/metadata/refresh-all", h.handleRefreshAll)
	mux.HandleFunc("POST /api/v1/admin/metadata/enrich-people", ratelimit.MiddlewareFunc(heavyRL, h.handleEnrichPeople))
}

func registerUserAdminRoutes(mux *http.ServeMux, userSvc *appuser.UserService, userQuery *appuser.QueryService, tokenGen *auth.JWTGenerator, actCmdSvc *appactivity.CommandService) {
	h := &adminHandler{tokenGen: tokenGen, userSvc: userSvc, userQuery: userQuery, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("GET /api/v1/admin/users/{id}/bitrate-limit", h.handleGetBitrateLimit)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/bitrate-limit", h.handleSetBitrateLimit)
}

func registerLogsRoute(mux *http.ServeMux, logDir string, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, logDir: logDir}
	mux.HandleFunc("GET /api/v1/admin/logs", h.handleGetLogs)
	mux.HandleFunc("GET /api/v1/admin/logs/stream", h.handleStreamLogs)
}

func registerSystemStatsRoute(mux *http.ServeMux, startTime time.Time, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, startTime: startTime}
	mux.HandleFunc("GET /api/v1/admin/system/stats", h.handleSystemStats)
}

func registerServerConfigRoutes(mux *http.ServeMux, cfgSvc *appconfig.Service, tokenGen *auth.JWTGenerator, actCmdSvc *appactivity.CommandService) {
	h := &adminHandler{tokenGen: tokenGen, cfgSvc: cfgSvc, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("GET /api/v1/admin/server-config", h.handleListServerConfig)
	mux.HandleFunc("PUT /api/v1/admin/server-config/{key}", h.handleSetServerConfig)
	mux.HandleFunc("DELETE /api/v1/admin/server-config/{key}", h.handleDeleteServerConfig)
}

func registerTranscodeProfileRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator, actCmdSvc *appactivity.CommandService) {
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("POST /api/v1/admin/transcoding/profiles", h.handleCreateTranscodeProfile)
	mux.HandleFunc("PUT /api/v1/admin/transcoding/profiles/{id}", h.handleUpdateTranscodeProfile)
	mux.HandleFunc("DELETE /api/v1/admin/transcoding/profiles/{id}", h.handleDeleteTranscodeProfile)
}

func registerTranscodeCacheRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator, mediaGetter MediaItemGetter, actCmdSvc *appactivity.CommandService) {
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc, mediaGetter: mediaGetter, activityCmdSvc: actCmdSvc}
	mux.HandleFunc("GET /api/v1/admin/transcode-cache", h.handleGetTranscodeCache)
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache", h.handleClearTranscodeCache)
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache/{filename}", h.handleDeleteTranscodeCacheFile)
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
