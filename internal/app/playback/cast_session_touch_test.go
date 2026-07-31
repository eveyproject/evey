package playback_test

import (
	"context"
	"testing"
	"time"

	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/domain/playback"
	playbackfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TouchCastSession is the TV-driven session keepalive: a cast receiver's
// liveness beacon refreshes the active session of (profile, media) - position
// AND updated_at - so a phone-orphaned cast neither vanishes from the
// now-playing panel nor gets reaped as stale while the TV plays.

func newTouchSvc(t *testing.T) (*appplayback.CommandService, *playbackfakes.FakePlaySessionRepository) {
	t.Helper()
	sessions := playbackfakes.NewFakePlaySessionRepository()
	progress := playbackfakes.NewFakeWatchProgressRepository()
	users := userfakes.NewFakeUserRepository()
	profiles := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	return appplayback.NewCommandService(progress, sessions, sessions, users, profiles, bus), sessions
}

func activeSession(id, profileID, mediaID, device string) *playback.PlaySession {
	return &playback.PlaySession{
		ID:             id,
		ProfileID:      shared.ProfileID(profileID),
		MediaItemID:    shared.MediaItemID(mediaID),
		StartedAt:      time.Now().Add(-time.Minute),
		CastDeviceName: device,
	}
}

func TestTouchCastSession_RefreshesTheActiveSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "Salon TV")))

	touched, err := svc.TouchCastSession(context.Background(), "prof-1", "media-1", 214_200)
	require.NoError(t, err)
	assert.True(t, touched)

	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	require.NotNil(t, got.Position)
	assert.Equal(t, int64(214_200), got.Position.Milliseconds(), "beacon position lands on the session")
	assert.Equal(t, "Salon TV", got.CastDeviceName, "the device name the phone set is preserved")
	assert.False(t, got.IsPaused, "an alive-beating receiver is playing")
}

// Superseded 2026-07-29: a positioned beacon with no session now RESURRECTS
// one (see TestTouchCastSession_ResurrectsWhenNoActiveSession) - the no-op
// contract hid a real playback from the panel in prod.

func TestTouchCastSession_IgnoresOtherMediaOfTheProfile(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-OTHER", "")))

	touched, err := svc.TouchCastSession(context.Background(), "prof-1", "media-1", 5_000)
	require.NoError(t, err)
	assert.True(t, touched, "resurrects a session for media-1 alongside")

	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Nil(t, got.Position, "the other media's session is untouched")
}

// ConsumeCastCommand routes an admin session command (pause/play/stop/message)
// to the TV itself: the receiver's alive beacon polls it, so a frozen phone no
// longer takes the admin's remote control down with it. Destructive read, same
// as the phone's ReportProgress consume path.
func TestConsumeCastCommand_DeliversThePendingCommandOnce(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "Salon TV")))
	require.NoError(t, svc.SetRemoteCommand(context.Background(), "s-1", "pause"))

	cmd, err := svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Equal(t, "pause", cmd)

	cmd, err = svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Empty(t, cmd, "a consumed command does not replay")
}

func TestConsumeCastCommand_NoSessionOrNoCommand(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	cmd, err := svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Empty(t, cmd)

	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "")))
	cmd, err = svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Empty(t, cmd)
}

// SetCastAudioPath mirrors the receiver's audio_mode beacon onto the session
// row so the admin panel's badge tells the truth in near real time: the AAC
// leg IS a server transcode, the wasm leg is TV-side decode (the server's
// ffmpeg dies at idle seconds later).
func TestSetCastAudioPath_MapsModesOntoTheSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "Salon TV")))

	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "wasm"))
	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "wasm", got.AudioDecision)

	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "transcode", got.AudioDecision, "the AAC leg is a server transcode")

	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac-latched"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "transcode", got.AudioDecision)

	// Unknown mode strings from a future receiver: ignored, never written.
	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "weird"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "transcode", got.AudioDecision)

	// No active session: a quiet no-op.
	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-OTHER", "wasm"))
}

// Resurrection: prod 2026-07-29 - the phone re-cast from an already-mounted
// player (no new StartStream) after its previous session had been reaped; the
// TV played for real while "now playing" stayed empty and nothing was
// pilotable. A positioned liveness beacon proves a genuine playback pinned to
// (profile, media): create the minimal session row; the reaper ends it when
// the beacons stop.
func TestTouchCastSession_ResurrectsWhenNoActiveSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)

	touched, err := svc.TouchCastSession(context.Background(), "prof-1", "media-1", 214_200)
	require.NoError(t, err)
	assert.True(t, touched, "a positioned beacon with no session must create one")

	active, err := sessions.GetActiveByProfile(context.Background(), "prof-1")
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "media-1", string(active[0].MediaItemID))
	assert.Equal(t, "Chromecast", active[0].DeviceName)
	assert.Equal(t, "Chromecast", active[0].CastDeviceName)
	require.NotNil(t, active[0].Position)
	assert.Equal(t, int64(214_200), active[0].Position.Milliseconds())

	// The next beacon touches the SAME session (no duplicate rows).
	touched, err = svc.TouchCastSession(context.Background(), "prof-1", "media-1", 244_200)
	require.NoError(t, err)
	assert.True(t, touched)
	active, err = sessions.GetActiveByProfile(context.Background(), "prof-1")
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

// An unusable beacon must be a silent no-op: no row, and no error to the
// caller (the receiver keeps beaconing either way). Nothing pinned that
// contract before.
//
// It now holds twice over, which was the point of routing the resurrect path
// through NewPlaySession: deleting the early-return guard at the top of
// TouchCastSession keeps this test GREEN, because the constructor's Validate()
// refuses the same rows. Verified by removing that guard and re-running. With
// the previous struct literal, neither layer existed once the guard was gone.
func TestTouchCastSession_IgnoresAnUnusableBeacon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		profileID  string
		mediaID    string
		positionMs int64
	}{
		{"no profile", "", "media-1", 214_200},
		{"no media", "prof-1", "", 214_200},
		{"negative position", "prof-1", "media-1", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, sessions := newTouchSvc(t)

			touched, err := svc.TouchCastSession(context.Background(), tc.profileID, tc.mediaID, tc.positionMs)
			require.NoError(t, err, "an unusable beacon is ignored, never an error to the caller")
			assert.False(t, touched, "nothing was resurrected")

			active, err := sessions.GetActiveByProfile(context.Background(), tc.profileID)
			require.NoError(t, err)
			assert.Empty(t, active, "no row may reach the store")
		})
	}
}
