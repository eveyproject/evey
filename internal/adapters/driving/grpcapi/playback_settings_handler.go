package grpcapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"sync"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appconfig "github.com/gravity-zero/evey/internal/app/config"
	"github.com/gravity-zero/evey/internal/config"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
)

const (
	cfgKeySkipForward  = "playback.skip_forward_seconds"
	cfgKeySkipBackward = "playback.skip_backward_seconds"
)

type playbackSettingsHandler struct {
	eveyv1.UnimplementedPlaybackSettingsServiceServer
	mu         sync.RWMutex
	cfg        *config.Config
	configSvc  *appconfig.Service
	translator *eveyi18n.Translator
}

// initFromDB loads persisted playback settings into the in-memory config cache.
// Falls back to config defaults if no DB entries exist.
func (h *playbackSettingsHandler) initFromDB(ctx context.Context) {
	if h.configSvc == nil {
		return
	}
	if e, err := h.configSvc.Get(ctx, cfgKeySkipForward); err == nil {
		if v, err := strconv.Atoi(e.Value); err == nil && v > 0 {
			h.cfg.Playback.SkipForwardSeconds = v
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("load playback skip_forward from db", "err", err)
	}
	if e, err := h.configSvc.Get(ctx, cfgKeySkipBackward); err == nil {
		if v, err := strconv.Atoi(e.Value); err == nil && v > 0 {
			h.cfg.Playback.SkipBackwardSeconds = v
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("load playback skip_backward from db", "err", err)
	}
}

func (h *playbackSettingsHandler) GetPlaybackSettings(_ context.Context, _ *eveyv1.GetPlaybackSettingsRequest) (*eveyv1.PlaybackSettings, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return &eveyv1.PlaybackSettings{
		SkipForwardSeconds:  int32(h.cfg.Playback.SkipForwardSeconds),
		SkipBackwardSeconds: int32(h.cfg.Playback.SkipBackwardSeconds),
	}, nil
}

func (h *playbackSettingsHandler) UpdatePlaybackSettings(ctx context.Context, req *eveyv1.UpdatePlaybackSettingsRequest) (*eveyv1.PlaybackSettings, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if req.SkipForwardSeconds != nil && *req.SkipForwardSeconds > 0 {
		h.cfg.Playback.SkipForwardSeconds = int(*req.SkipForwardSeconds)
	}
	if req.SkipBackwardSeconds != nil && *req.SkipBackwardSeconds > 0 {
		h.cfg.Playback.SkipBackwardSeconds = int(*req.SkipBackwardSeconds)
	}
	if h.cfg.Playback.SkipForwardSeconds <= 0 {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.playback_settings.skip_forward_positive")
	}

	if h.configSvc != nil {
		if err := h.configSvc.Set(ctx, cfgKeySkipForward, strconv.Itoa(h.cfg.Playback.SkipForwardSeconds)); err != nil {
			slog.Error("persist playback skip_forward", "err", err)
		}
		if err := h.configSvc.Set(ctx, cfgKeySkipBackward, strconv.Itoa(h.cfg.Playback.SkipBackwardSeconds)); err != nil {
			slog.Error("persist playback skip_backward", "err", err)
		}
	}

	return &eveyv1.PlaybackSettings{
		SkipForwardSeconds:  int32(h.cfg.Playback.SkipForwardSeconds),
		SkipBackwardSeconds: int32(h.cfg.Playback.SkipBackwardSeconds),
	}, nil
}
