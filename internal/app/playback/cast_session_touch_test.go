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

func TestTouchCastSession_NoActiveSessionIsANoOp(t *testing.T) {
	t.Parallel()
	svc, _ := newTouchSvc(t)
	touched, err := svc.TouchCastSession(context.Background(), "prof-1", "media-1", 5_000)
	require.NoError(t, err)
	assert.False(t, touched)
}

func TestTouchCastSession_IgnoresOtherMediaOfTheProfile(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-OTHER", "")))

	touched, err := svc.TouchCastSession(context.Background(), "prof-1", "media-1", 5_000)
	require.NoError(t, err)
	assert.False(t, touched)

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
