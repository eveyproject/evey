package playback_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reaching a Chromecast that is still beating but has no session left, through
// the service - see cast_orphan_commands.go for why the profile is the key.
func TestConsumeCastCommand_FallsBackToTheOrphanStore(t *testing.T) {
	t.Parallel()
	svc, _ := newTouchSvc(t)

	// No session at all: exactly the state a stalled receiver ends up in once
	// its session has aged out.
	require.NoError(t, svc.SetCastReceiverCommand(context.Background(), "prof-1", "stop"))

	cmd, err := svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Equal(t, "stop", cmd, "the beacon is the only channel left to that television")

	cmd, err = svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Empty(t, cmd, "and it is handed over exactly once")
}

// Any media id: the receiver reports whatever it last loaded, and a session
// that no longer exists cannot be matched on. The profile is the key.
func TestConsumeCastCommand_OrphanFallbackIgnoresTheMediaID(t *testing.T) {
	t.Parallel()
	svc, _ := newTouchSvc(t)
	require.NoError(t, svc.SetCastReceiverCommand(context.Background(), "prof-1", "stop"))

	cmd, err := svc.ConsumeCastCommand(context.Background(), "prof-1", "some-other-title")
	require.NoError(t, err)
	assert.Equal(t, "stop", cmd)
}

// The session path still wins while a session exists, so an admin acting on a
// live now-playing row keeps the behaviour they had.
func TestConsumeCastCommand_SessionCommandTakesPrecedence(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "Salon TV")))
	require.NoError(t, svc.SetRemoteCommand(context.Background(), "s-1", "pause"))
	require.NoError(t, svc.SetCastReceiverCommand(context.Background(), "prof-1", "stop"))

	cmd, err := svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Equal(t, "pause", cmd, "the session's own command comes first")

	// The parked one is still there for the next beat rather than lost.
	cmd, err = svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Equal(t, "stop", cmd)
}

func TestSetCastReceiverCommand_RefusesEmptyInput(t *testing.T) {
	t.Parallel()
	svc, _ := newTouchSvc(t)
	require.Error(t, svc.SetCastReceiverCommand(context.Background(), "", "stop"))
	require.Error(t, svc.SetCastReceiverCommand(context.Background(), "prof-1", ""))
}
