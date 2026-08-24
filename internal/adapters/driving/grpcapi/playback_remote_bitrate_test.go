package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
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

// A profile's SAVED playback settings must actually reach the decision.
//
// resolvePlaybackPrefs loads the caller's profile and takes two things from
// it: the parsed transcoding preferences (preferred codecs, channel ceiling,
// direct-play toggles) and MaxBitrateKbps, which is folded in as a MIN with
// whatever the client asked for. No test in this package ever seeded a profile
// carrying either, so the whole branch under `if h.userQuery != nil` was dead
// weight in the suite: drop the profile lookup and everything still passed.
//
// The symptom of a regression is the worst kind - the app looks fine, the
// stream plays, and the setting the viewer saved simply stops applying.
func TestResolvePlaybackPrefs_ProfileSettingsApply(t *testing.T) {
	newHandler := func(prof *user.Profile) *playbackHandler {
		profiles := userfakes.NewFakeProfileRepository()
		if prof != nil {
			_ = profiles.Create(context.Background(), prof)
		}
		return &playbackHandler{
			userQuery: appuser.NewQueryService(
				userfakes.NewFakeUserRepository(), profiles,
				userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
				userfakes.NewFakeQuickConnectRepository(),
			),
		}
	}
	streams := []*library.MediaStream{}
	ctxFor := func(profileID string) context.Context {
		return interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "u-1", ProfileID: profileID})
	}

	cap2000 := 2000
	prof := &user.Profile{
		ID:     "prf-prefs",
		UserID: "u-1",
		Name:   "Viewer",
		Preferences: map[string]string{
			"preferred_video_codec": "hevc",
			"max_audio_channels":    "2",
		},
		MaxBitrateKbps: &cap2000,
	}

	t.Run("saved preferences reach the decision", func(t *testing.T) {
		prefs, _ := newHandler(prof).resolvePlaybackPrefs(ctxFor("prf-prefs"), &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, "hevc", prefs.PreferredVideoCodec, "the profile's codec preference must survive the request merge")
		assert.Equal(t, 2, prefs.MaxAudioChannels)
	})

	t.Run("the profile cap applies when the client asks for nothing", func(t *testing.T) {
		_, mb := newHandler(prof).resolvePlaybackPrefs(ctxFor("prf-prefs"), &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 2000, mb)
	})

	t.Run("the profile cap only LOWERS - a tighter client request wins", func(t *testing.T) {
		// req.MaxBitrate is bits/s: 1_000_000 = 1000 kbps, below the 2000 cap.
		_, mb := newHandler(prof).resolvePlaybackPrefs(ctxFor("prf-prefs"),
			&eveyv1.GetStreamInfoRequest{MaxBitrate: 1_000_000}, streams)
		assert.Equal(t, 1000, mb, "the profile ceiling must not raise a client that asked for less")
	})

	t.Run("a looser client request is clamped down to the profile cap", func(t *testing.T) {
		_, mb := newHandler(prof).resolvePlaybackPrefs(ctxFor("prf-prefs"),
			&eveyv1.GetStreamInfoRequest{MaxBitrate: 8_000_000}, streams)
		assert.Equal(t, 2000, mb)
	})

	t.Run("the request's network type survives the profile merge", func(t *testing.T) {
		prefs, _ := newHandler(prof).resolvePlaybackPrefs(ctxFor("prf-prefs"),
			&eveyv1.GetStreamInfoRequest{NetworkType: "cellular"}, streams)
		assert.Equal(t, "cellular", prefs.NetworkType,
			"network type comes from the REQUEST, not the profile - the profile parse must not overwrite it")
	})
}
