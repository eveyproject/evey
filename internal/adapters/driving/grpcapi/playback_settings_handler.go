package grpcapi

import (
	"context"
	"sync"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/config"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
)

type playbackSettingsHandler struct {
	eveyv1.UnimplementedPlaybackSettingsServiceServer
	mu         sync.RWMutex
	cfg        *config.Config
	translator *eveyi18n.Translator
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
	return &eveyv1.PlaybackSettings{
		SkipForwardSeconds:  int32(h.cfg.Playback.SkipForwardSeconds),
		SkipBackwardSeconds: int32(h.cfg.Playback.SkipBackwardSeconds),
	}, nil
}
