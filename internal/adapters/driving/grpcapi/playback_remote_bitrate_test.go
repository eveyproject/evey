package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolvePlaybackPrefs_RemoteBitrateCap covers the operator-wide
// remote-access bitrate cap wired into GetStreamInfo: a WAN session is clamped
// to maxRemoteBitrateKbps, a LAN session is untouched, the cap only ever LOWERS
// the client/profile ceiling, and an unresolved client IP fails open.
func TestResolvePlaybackPrefs_RemoteBitrateCap(t *testing.T) {
	h := &playbackHandler{
		remoteAccess: func() (int, []string) { return 4000, []string{"10.0.0.0/8"} },
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
		off := &playbackHandler{remoteAccess: func() (int, []string) { return 0, nil }}
		ctx := interceptors.WithClientIP(context.Background(), "203.0.113.7")
		_, mb := off.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 0, mb)
	})

	// Admin -> Remote Access writes the live config; the next playback reads
	// it. The cap used to be copied at boot, so a change needed a restart.
	t.Run("a changed cap and LAN list apply to the next playback", func(t *testing.T) {
		remoteCap, lan := 4000, []string{"10.0.0.0/8"}
		live := &playbackHandler{remoteAccess: func() (int, []string) { return remoteCap, lan }}
		ctx := interceptors.WithClientIP(context.Background(), "203.0.113.7")
		_, mb := live.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 4000, mb)

		remoteCap = 1500
		_, mb = live.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 1500, mb)

		lan = append(lan, "203.0.113.0/24")
		_, mb = live.resolvePlaybackPrefs(ctx, &eveyv1.GetStreamInfoRequest{}, streams)
		assert.Equal(t, 0, mb, "an address added to the LAN list is no longer capped")
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

// servedResolution is what the "Playing in 1080p" badge reads, and nothing
// tested it directly: the transcode cap wins, the source height is the
// fallback, and no height at all must render an EMPTY string rather than
// "0p" - a badge reading "0p" is worse than no badge.
func TestServedResolution(t *testing.T) {
	h1080, h2160 := 1080, 2160
	video := func(h *int) []*library.MediaStream {
		return []*library.MediaStream{{StreamType: library.StreamTypeVideo, Height: h}}
	}

	t.Run("the transcode cap wins over the source", func(t *testing.T) {
		got := servedResolution(&transcoding.PlaybackDecision{MaxHeight: 720}, video(&h2160))
		assert.Equal(t, "720p", got, "a 4K source transcoded to 720p is being SERVED at 720p")
	})

	t.Run("no cap falls back to the source height", func(t *testing.T) {
		assert.Equal(t, "1080p", servedResolution(&transcoding.PlaybackDecision{}, video(&h1080)))
	})

	t.Run("a source with no usable height renders nothing, never 0p", func(t *testing.T) {
		assert.Empty(t, servedResolution(&transcoding.PlaybackDecision{}, video(nil)))
		assert.Empty(t, servedResolution(&transcoding.PlaybackDecision{}, nil))
		zero := 0
		assert.Empty(t, servedResolution(&transcoding.PlaybackDecision{}, video(&zero)))
	})

	t.Run("the first video stream decides - audio streams are skipped", func(t *testing.T) {
		streams := []*library.MediaStream{
			{StreamType: library.StreamTypeAudio, Height: &h2160},
			{StreamType: library.StreamTypeVideo, Height: &h1080},
		}
		assert.Equal(t, "1080p", servedResolution(&transcoding.PlaybackDecision{}, streams))
	})
}

// GetWatchProgress answers an EMPTY progress - not an error - when the caller
// may not see the media. That is deliberate: an empty body is indistinguishable
// from "never watched", so it leaks nothing, where a PermissionDenied would
// confirm the id exists. Its write-side siblings (MarkAsWatched /
// MarkAsUnwatched) propagate the refusal instead, because there is nothing to
// hide once you are asking to CHANGE state.
//
// Nothing asserted either half, so the asymmetry read like an oversight. It
// is a decision; this test is what makes it one.
func TestGetWatchProgress_RefusalIsAnEmptyProgressNotAnError(t *testing.T) {
	profiles := userfakes.NewFakeProfileRepository()
	h := &playbackHandler{
		userQuery: appuser.NewQueryService(
			userfakes.NewFakeUserRepository(), profiles,
			userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
			userfakes.NewFakeQuickConnectRepository(),
		),
		// libQuery/userAccess left nil: requireMediaAccess then refuses, which
		// is exactly the "caller may not see this" branch under test.
	}
	ctx := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "u-1", ProfileID: "prf-1"})

	resp, err := h.GetWatchProgress(ctx, &eveyv1.GetWatchProgressRequest{MediaItemId: "mi-secret"})

	require.NoError(t, err, "a refusal here must NOT surface as an error - that would confirm the id exists")
	require.NotNil(t, resp)
	assert.Zero(t, resp.PositionMs, "the refusal must be indistinguishable from 'never watched'")
	assert.False(t, resp.Played)
	assert.Empty(t, resp.MediaItemId, "an empty progress carries no id either - nothing confirms the media")
}
