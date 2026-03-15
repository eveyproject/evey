package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/config"
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
