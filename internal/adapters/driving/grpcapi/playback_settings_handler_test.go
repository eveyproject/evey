package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestPlaybackSettingsHandler() *playbackSettingsHandler {
	cfg := &config.Config{
		Playback: config.PlaybackConfig{
			SkipForwardSeconds:  10,
			SkipBackwardSeconds: 10,
		},
	}
	return &playbackSettingsHandler{cfg: cfg}
}

func newTestPlaybackSettingsHandlerWithRepo() (*playbackSettingsHandler, *fakes.FakeServerConfigRepository) {
	cfg := &config.Config{
		Playback: config.PlaybackConfig{
			SkipForwardSeconds:  10,
			SkipBackwardSeconds: 10,
		},
	}
	repo := fakes.NewFakeServerConfigRepository()
	svc := appconfig.NewService(repo)
	return &playbackSettingsHandler{cfg: cfg, configSvc: svc}, repo
}

func TestPlaybackSettingsHandler_Get(t *testing.T) {
	h := newTestPlaybackSettingsHandler()

	resp, err := h.GetPlaybackSettings(context.Background(), &eveyv1.GetPlaybackSettingsRequest{})

	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.SkipForwardSeconds)
	assert.Equal(t, int32(10), resp.SkipBackwardSeconds)
}

func TestPlaybackSettingsHandler_Update_ForwardSeconds(t *testing.T) {
	h := newTestPlaybackSettingsHandler()
	fwd := int32(30)

	resp, err := h.UpdatePlaybackSettings(context.Background(), &eveyv1.UpdatePlaybackSettingsRequest{
		SkipForwardSeconds: &fwd,
	})

	require.NoError(t, err)
	assert.Equal(t, int32(30), resp.SkipForwardSeconds)
	assert.Equal(t, int32(10), resp.SkipBackwardSeconds)
}

func TestPlaybackSettingsHandler_Update_BackwardSeconds(t *testing.T) {
	h := newTestPlaybackSettingsHandler()
	bwd := int32(15)

	resp, err := h.UpdatePlaybackSettings(context.Background(), &eveyv1.UpdatePlaybackSettingsRequest{
		SkipBackwardSeconds: &bwd,
	})

	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.SkipForwardSeconds)
	assert.Equal(t, int32(15), resp.SkipBackwardSeconds)
}

func TestPlaybackSettingsHandler_Update_ZeroForwardIgnored(t *testing.T) {
	h := newTestPlaybackSettingsHandler()
	zero := int32(0)

	resp, err := h.UpdatePlaybackSettings(context.Background(), &eveyv1.UpdatePlaybackSettingsRequest{
		SkipForwardSeconds: &zero,
	})

	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.SkipForwardSeconds)
}

func TestPlaybackSettingsHandler_Update_InvalidArgument(t *testing.T) {
	cfg := &config.Config{
		Playback: config.PlaybackConfig{
			SkipForwardSeconds:  0,
			SkipBackwardSeconds: 10,
		},
	}
	h := &playbackSettingsHandler{cfg: cfg}
	fwd := int32(0)

	_, err := h.UpdatePlaybackSettings(context.Background(), &eveyv1.UpdatePlaybackSettingsRequest{
		SkipForwardSeconds: &fwd,
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPlaybackSettingsHandler_GetReflectsUpdate(t *testing.T) {
	h := newTestPlaybackSettingsHandler()
	fwd := int32(45)

	_, err := h.UpdatePlaybackSettings(context.Background(), &eveyv1.UpdatePlaybackSettingsRequest{
		SkipForwardSeconds: &fwd,
	})
	require.NoError(t, err)

	resp, err := h.GetPlaybackSettings(context.Background(), &eveyv1.GetPlaybackSettingsRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(45), resp.SkipForwardSeconds)
}

func TestPlaybackSettingsHandler_UpdatePersistsToRepo(t *testing.T) {
	h, repo := newTestPlaybackSettingsHandlerWithRepo()
	fwd := int32(25)
	bwd := int32(5)

	_, err := h.UpdatePlaybackSettings(context.Background(), &eveyv1.UpdatePlaybackSettingsRequest{
		SkipForwardSeconds:  &fwd,
		SkipBackwardSeconds: &bwd,
	})
	require.NoError(t, err)

	ctx := context.Background()
	e, err := repo.Get(ctx, appconfig.KeyPlaybackSkipForward)
	require.NoError(t, err)
	assert.Equal(t, "25", e.Value)

	e, err = repo.Get(ctx, appconfig.KeyPlaybackSkipBackward)
	require.NoError(t, err)
	assert.Equal(t, "5", e.Value)
}

func TestPlaybackSettingsHandler_InitFromDB(t *testing.T) {
	cfg := &config.Config{
		Playback: config.PlaybackConfig{
			SkipForwardSeconds:  10,
			SkipBackwardSeconds: 10,
		},
	}
	repo := fakes.NewFakeServerConfigRepository()
	ctx := context.Background()
	_ = repo.Set(ctx, appconfig.KeyPlaybackSkipForward, "60")
	_ = repo.Set(ctx, appconfig.KeyPlaybackSkipBackward, "20")

	svc := appconfig.NewService(repo)
	h := &playbackSettingsHandler{cfg: cfg, configSvc: svc}
	h.initFromDB(ctx)

	resp, err := h.GetPlaybackSettings(ctx, &eveyv1.GetPlaybackSettingsRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(60), resp.SkipForwardSeconds)
	assert.Equal(t, int32(20), resp.SkipBackwardSeconds)
}

func TestPlaybackSettingsHandler_InitFromDB_FallsBackToDefaults(t *testing.T) {
	cfg := &config.Config{
		Playback: config.PlaybackConfig{
			SkipForwardSeconds:  10,
			SkipBackwardSeconds: 10,
		},
	}
	repo := fakes.NewFakeServerConfigRepository()
	svc := appconfig.NewService(repo)
	h := &playbackSettingsHandler{cfg: cfg, configSvc: svc}
	h.initFromDB(context.Background())

	resp, err := h.GetPlaybackSettings(context.Background(), &eveyv1.GetPlaybackSettingsRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.SkipForwardSeconds)
	assert.Equal(t, int32(10), resp.SkipBackwardSeconds)
}
