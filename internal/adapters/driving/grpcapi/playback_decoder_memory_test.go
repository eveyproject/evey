package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/domain/user"
)

// A TV's first verdict after a cold start declared the baseline while its AC-3
// decoder check was still running: the server adds what the same box's
// previous check reported, so an AC-3 file goes out as it is.
func TestCompleteAudioDeclaration_AddsTheDevicesCheckedCodecs(t *testing.T) {
	const device = "EveyTV/0.2.11 (Android TV 7.1.1; Freebox Player Mini v2)"
	mem := apptranscoding.NewDeviceDecoderMemory()
	mem.ObservePlayerEvent("usr-1", device, "ac3go_check", "declared=ac3,eac3 blocked=no")
	h := &playbackHandler{deviceDecoders: mem}
	ctx := interceptors.WithUserAgent(interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "usr-1"}), device)

	client := transcoding.ClientProfile{AudioCodecs: []string{"aac", "mp3", "opus", "flac", "vorbis"}}
	h.completeAudioDeclaration(ctx, "media-1", &client)
	assert.Equal(t, []string{"aac", "mp3", "opus", "flac", "vorbis", "ac3", "eac3"}, client.AudioCodecs)

	other := transcoding.ClientProfile{AudioCodecs: []string{"aac"}}
	h.completeAudioDeclaration(interceptors.WithUserAgent(interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "usr-2"}), device), "media-1", &other)
	assert.Equal(t, []string{"aac"}, other.AudioCodecs, "another user's box is another device")

	unwired := transcoding.ClientProfile{AudioCodecs: []string{"aac"}}
	(&playbackHandler{}).completeAudioDeclaration(ctx, "media-1", &unwired)
	assert.Equal(t, []string{"aac"}, unwired.AudioCodecs)
}
