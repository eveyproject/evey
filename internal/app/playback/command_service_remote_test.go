package playback_test

import (
	"context"
	"testing"

	appplayback "github.com/eveyproject/evey/internal/app/playback"
	playbackfakes "github.com/eveyproject/evey/internal/domain/playback/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
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

	initDone := make(chan struct{})
	svc.StartCleanup(ctx, initDone)
	<-initDone
	cancel()
	// A second call with an already-cancelled ctx also exits cleanly -
	// the goroutine runs the initial cleanup before checking ctx in the
	// loop, so initDone fires once even on a pre-cancelled ctx.
	initDone2 := make(chan struct{})
	svc.StartCleanup(ctx, initDone2)
	<-initDone2
}
