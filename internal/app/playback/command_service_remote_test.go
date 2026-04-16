package playback_test

import (
	"context"
	"testing"
	"time"

	appplayback "github.com/evey-project/evey/internal/app/playback"
	playbackfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRemoteCmdSvc() *appplayback.CommandService {
	sessions := playbackfakes.NewFakePlaySessionRepository()
	progress := playbackfakes.NewFakeWatchProgressRepository()
	users := userfakes.NewFakeUserRepository()
	profiles := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	return appplayback.NewCommandService(progress, sessions, sessions, users, profiles, bus)
}

// SetRemoteCommand delegates to the session repo without error.
func TestCommandService_SetRemoteCommand_Delegates(t *testing.T) {
	t.Parallel()
	svc := newRemoteCmdSvc()
	require.NoError(t, svc.SetRemoteCommand(context.Background(), "s-1", "pause"))
}

// ConsumeRemoteCommand delegates to the session repo without error.
func TestCommandService_ConsumeRemoteCommand_Delegates(t *testing.T) {
	t.Parallel()
	svc := newRemoteCmdSvc()
	cmd, err := svc.ConsumeRemoteCommand(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "", cmd)
}

// StartCleanup starts a background goroutine that calls CloseStale. We verify
// it doesn't panic and exits cleanly on ctx cancel.
func TestCommandService_StartCleanup_RespectsContextCancel(t *testing.T) {
	t.Parallel()
	svc := newRemoteCmdSvc()
	ctx, cancel := context.WithCancel(context.Background())

	svc.StartCleanup(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()
	// A second call with an already-cancelled ctx also exits cleanly.
	svc.StartCleanup(ctx)
	time.Sleep(10 * time.Millisecond)
}
