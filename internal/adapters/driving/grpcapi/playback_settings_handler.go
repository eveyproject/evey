package grpcapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	"github.com/eveyproject/evey/internal/config"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
)

type playbackSettingsHandler struct {
	eveyv1.UnimplementedPlaybackSettingsServiceServer
	// No private mutex: the REST handler writes these same fields, so the
	// config's own lock is the only one that excludes anything.
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
	// Read from the DB first, apply under the config lock after. Boot usually
	// runs before anything serves, but "usually" is not a synchronization
	// argument: these are the same fields the two settings handlers write.
	var fwd, bwd int
	if e, err := h.configSvc.Get(ctx, appconfig.KeyPlaybackSkipForward); err == nil {
		if v, err := strconv.Atoi(e.Value); err == nil && v > 0 {
			fwd = v
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.WarnContext(ctx, "load playback skip_forward from db", "err", err)
	}
	if e, err := h.configSvc.Get(ctx, appconfig.KeyPlaybackSkipBackward); err == nil {
		if v, err := strconv.Atoi(e.Value); err == nil && v > 0 {
			bwd = v
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.WarnContext(ctx, "load playback skip_backward from db", "err", err)
	}
	if fwd == 0 && bwd == 0 {
		return // nothing persisted: keep the config defaults
	}
	h.cfg.ApplyServerSettings(func() {
		if fwd > 0 {
			h.cfg.Playback.SkipForwardSeconds = fwd
		}
		if bwd > 0 {
			h.cfg.Playback.SkipBackwardSeconds = bwd
		}
	})
}

func (h *playbackSettingsHandler) GetPlaybackSettings(ctx context.Context, _ *eveyv1.GetPlaybackSettingsRequest) (*eveyv1.PlaybackSettings, error) {
	_, span := tracing.StartSpan(ctx, "grpc.playback_settings.GetPlaybackSettings")
	defer span.End()
	fwd, bwd := h.cfg.EffectivePlaybackSkipSeconds()
	return &eveyv1.PlaybackSettings{
		SkipForwardSeconds:  int32(fwd),
		SkipBackwardSeconds: int32(bwd),
	}, nil
}

func (h *playbackSettingsHandler) UpdatePlaybackSettings(ctx context.Context, req *eveyv1.UpdatePlaybackSettingsRequest) (*eveyv1.PlaybackSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback_settings.UpdatePlaybackSettings")
	defer span.End()
	var fwd, bwd int
	h.cfg.ApplyServerSettings(func() {
		if req.SkipForwardSeconds != nil && *req.SkipForwardSeconds > 0 {
			h.cfg.Playback.SkipForwardSeconds = int(*req.SkipForwardSeconds)
		}
		if req.SkipBackwardSeconds != nil && *req.SkipBackwardSeconds > 0 {
			h.cfg.Playback.SkipBackwardSeconds = int(*req.SkipBackwardSeconds)
		}
		fwd = h.cfg.Playback.SkipForwardSeconds
		bwd = h.cfg.Playback.SkipBackwardSeconds
	})
	if fwd <= 0 {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.playback_settings.skip_forward_positive")
	}

	// Persisted OUTSIDE the lock: two DB round-trips must not hold a lock that
	// every settings read waits on.
	if h.configSvc != nil {
		if err := h.configSvc.Set(ctx, appconfig.KeyPlaybackSkipForward, strconv.Itoa(fwd)); err != nil {
			slog.ErrorContext(ctx, "persist playback skip_forward", "err", err)
		}
		if err := h.configSvc.Set(ctx, appconfig.KeyPlaybackSkipBackward, strconv.Itoa(bwd)); err != nil {
			slog.ErrorContext(ctx, "persist playback skip_backward", "err", err)
		}
	}

	return &eveyv1.PlaybackSettings{
		SkipForwardSeconds:  int32(fwd),
		SkipBackwardSeconds: int32(bwd),
	}, nil
}
