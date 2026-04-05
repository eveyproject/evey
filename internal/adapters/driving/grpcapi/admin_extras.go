package grpcapi

import (
	"context"
	"log/slog"
	goruntime "runtime"
	"sync"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	statsMu      sync.Mutex
	cachedMS     goruntime.MemStats
	cacheExpires time.Time
)

func (h *adminHandler) GetSystemStats(_ context.Context, _ *eveyv1.GetSystemStatsRequest) (*eveyv1.SystemStats, error) {
	statsMu.Lock()
	if time.Now().After(cacheExpires) {
		goruntime.ReadMemStats(&cachedMS)
		cacheExpires = time.Now().Add(30 * time.Second)
	}
	ms := cachedMS
	statsMu.Unlock()

	return &eveyv1.SystemStats{
		Goroutines:    int32(goruntime.NumGoroutine()),
		MemAllocBytes: ms.Alloc,
		MemSysBytes:   ms.Sys,
		NumGc:         ms.NumGC,
		UptimeSeconds: int64(time.Since(h.startTime).Seconds()),
		GoVersion:     goruntime.Version(),
		Os:            goruntime.GOOS,
		Arch:          goruntime.GOARCH,
		NumCpu:        int32(goruntime.NumCPU()),
	}, nil
}

func (h *adminHandler) EnrichPeopleBios(_ context.Context, _ *eveyv1.EnrichPeopleBiosRequest) (*eveyv1.EnrichPeopleBiosResponse, error) {
	if h.bioEnricher == nil {
		return nil, status.Error(codes.Unavailable, "enricher not configured")
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in EnrichPeopleBios", slog.Any("recover", r))
			}
		}()
		count, err := h.bioEnricher.EnrichAllPeopleBios(h.serverCtx, slog.Default())
		if err != nil {
			slog.Error("enrich people bios", slog.Any("err", err))
		} else {
			slog.Info("enrich people bios completed", slog.Int("enriched", count))
		}
	}()
	return &eveyv1.EnrichPeopleBiosResponse{Message: "enrichment started"}, nil
}

func (h *adminHandler) GetActiveSessions(ctx context.Context, _ *eveyv1.GetActiveSessionsRequest) (*eveyv1.GetActiveSessionsResponse, error) {
	if h.sessionProvider == nil {
		return &eveyv1.GetActiveSessionsResponse{}, nil
	}
	sessions, err := h.sessionProvider.GetActiveSessions(ctx)
	if err != nil {
		slog.Error("active sessions", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "internal error")
	}
	resp := &eveyv1.GetActiveSessionsResponse{}
	for _, s := range sessions {
		as := &eveyv1.ActiveSession{
			SessionId:    s.SessionID,
			ProfileName:  s.ProfileName,
			MediaTitle:   s.MediaTitle,
			MediaType:    s.MediaType,
			MediaItemId:  s.MediaItemID,
			Progress:     s.ProgressPercent(),
			DurationMs:   s.DurationMs,
			PositionMs:   s.PositionMs,
			StartedAt:    timestamppb.New(s.StartedAt),
			DeviceName:   s.DeviceName,
			DeviceType:   s.DeviceType,
			ClientIp:     s.ClientIP,
			PlayMethod:   s.PlayMethod,
			VideoCodec:   s.VideoCodec,
			AudioCodec:   s.AudioCodec,
			Resolution:   s.Resolution,
			BandwidthBps: s.BandwidthBps,
		}
		if s.TranscodeInfo != nil {
			as.TranscodeInfo = s.TranscodeInfo
		}
		resp.Sessions = append(resp.Sessions, as)
	}
	return resp, nil
}

func (h *adminHandler) KillSession(ctx context.Context, req *eveyv1.KillSessionRequest) (*eveyv1.KillSessionResponse, error) {
	if h.sessionProvider == nil {
		return nil, status.Error(codes.Unimplemented, "session management not available")
	}
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id required")
	}
	if err := h.sessionProvider.StopStream(ctx, req.SessionId); err != nil {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	return &eveyv1.KillSessionResponse{}, nil
}

func (h *adminHandler) GetUserBitrateLimit(ctx context.Context, req *eveyv1.GetUserBitrateLimitRequest) (*eveyv1.GetUserBitrateLimitResponse, error) {
	if h.userQuery == nil {
		return nil, status.Error(codes.Unimplemented, "user service not available")
	}
	limit, err := h.userQuery.GetUserBitrateLimit(ctx, req.UserId)
	if err != nil {
		slog.Error("get user bitrate limit", slog.String("userID", req.UserId), slog.Any("err", err))
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &eveyv1.GetUserBitrateLimitResponse{MaxBitrateKbps: int32(limit)}, nil
}

func (h *adminHandler) SetUserBitrateLimit(ctx context.Context, req *eveyv1.SetUserBitrateLimitRequest) (*eveyv1.SetUserBitrateLimitResponse, error) {
	if h.userSvc == nil {
		return nil, status.Error(codes.Unimplemented, "user service not available")
	}
	if req.MaxBitrateKbps < 0 {
		return nil, status.Error(codes.InvalidArgument, "max_bitrate_kbps must be >= 0")
	}
	if err := h.userSvc.SetUserBitrateLimit(ctx, req.UserId, int(req.MaxBitrateKbps)); err != nil {
		slog.Error("set user bitrate limit", slog.String("userID", req.UserId), slog.Any("err", err))
		return nil, status.Error(codes.Internal, "internal error")
	}
	h.auditFromCtx(ctx, "set_bitrate_limit", "user", &req.UserId, map[string]any{"max_bitrate_kbps": req.MaxBitrateKbps})
	return &eveyv1.SetUserBitrateLimitResponse{MaxBitrateKbps: req.MaxBitrateKbps}, nil
}

func (h *adminHandler) auditFromCtx(ctx context.Context, action, resourceType string, resourceID *string, details map[string]any) {
	if h.activityCmd == nil {
		return
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return
	}
	ua := interceptors.UserAgentFromCtx(ctx)
	ip := interceptors.ClientIPFromCtx(ctx)
	e := &activity.AuditLogEntry{
		UserID:       claims.UserID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
	}
	if ua != "" {
		e.UserAgent = &ua
	}
	if ip != "" {
		e.ClientIP = &ip
	}
	if err := h.activityCmd.CreateAuditEntry(ctx, e); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}
