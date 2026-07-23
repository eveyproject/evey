package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/stretchr/testify/assert"
)

// TestResolvePlaybackPrefs_RemoteBitrateCap covers the operator-wide
// remote-access bitrate cap wired into GetStreamInfo: a WAN session is clamped
// to maxRemoteBitrateKbps, a LAN session is untouched, the cap only ever LOWERS
// the client/profile ceiling, and an unresolved client IP fails open.
func TestResolvePlaybackPrefs_RemoteBitrateCap(t *testing.T) {
	h := &playbackHandler{
		remoteNetDetector:    network.NewDetector([]string{"10.0.0.0/8"}),
		maxRemoteBitrateKbps: 4000,
	}
	streams := []*library.MediaStream{}

	t.Run("WAN session capped", func(t *testing.T) {
		ctx := interceptors.WithClientIP(context.Background(), "203.0.113.7")
		_, mb := h.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 4000, mb)
	})

	t.Run("configured-LAN session not capped", func(t *testing.T) {
		ctx := interceptors.WithClientIP(context.Background(), "10.1.2.3")
		_, mb := h.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 0, mb)
	})

	t.Run("private-range session not capped", func(t *testing.T) {
		ctx := interceptors.WithClientIP(context.Background(), "192.168.1.20")
		_, mb := h.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 0, mb)
	})

	t.Run("cap only lowers, never raises a tighter client bitrate", func(t *testing.T) {
		ctx := interceptors.WithClientIP(context.Background(), "203.0.113.7")
		// req.MaxBitrate is bits/s; 2_000_000 = 2000 kbps, below the 4000 cap.
		_, mb := h.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{MaxBitrate: 2_000_000}, streams)
		assert.Equal(t, 2000, mb)
	})

	t.Run("unresolved client IP fails open", func(t *testing.T) {
		_, mb := h.resolvePlaybackPrefs(context.Background(), &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 0, mb)
	})

	t.Run("cap disabled leaves bitrate untouched", func(t *testing.T) {
		off := &playbackHandler{remoteNetDetector: network.NewDetector(nil), maxRemoteBitrateKbps: 0}
		ctx := interceptors.WithClientIP(context.Background(), "203.0.113.7")
		_, mb := off.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 0, mb)
	})
}
