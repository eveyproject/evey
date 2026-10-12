package playback_test

import (
	"context"
	"testing"
	"time"

	appplayback "github.com/eveyproject/evey/internal/app/playback"
	"github.com/eveyproject/evey/internal/domain/playback"
	pbfakes "github.com/eveyproject/evey/internal/domain/playback/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Prod 2026-10-05: a restart ended a web session whose player kept streaming
// and ticking; every tick answered OK and the film never came back to
// now-playing. The first tick after the restart must reopen it.
func TestReportProgressBySession_ReopensSessionEndedByRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pr := pbfakes.NewFakeWatchProgressRepository()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	seedUserProfile(ctx, ur, pfr, nil)

	before := appplayback.NewCommandService(pr, sr, sr, ur, pfr, sharedfakes.NewFakeEventBus())
	require.NoError(t, before.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	n, err := before.EndAllSessions(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	time.Sleep(2 * time.Millisecond)
	after := appplayback.NewCommandService(pr, sr, sr, ur, pfr, sharedfakes.NewFakeEventBus())
	require.NoError(t, after.ReportProgressBySession(ctx, "s1", 2_100_000, false, ""))

	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.Nil(t, stored.EndedAt, "the tick after the restart must reopen the session")
	require.NotNil(t, stored.Position)
	assert.Equal(t, int64(2_100_000), stored.Position.Milliseconds())
}

// The symmetric case: a session stopped by THIS process stays stopped, even
// when the previous process had already ended and reopened it.
func TestReportProgressBySession_StopAfterReopenStaysEnded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pr := pbfakes.NewFakeWatchProgressRepository()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	seedUserProfile(ctx, ur, pfr, nil)

	before := appplayback.NewCommandService(pr, sr, sr, ur, pfr, sharedfakes.NewFakeEventBus())
	require.NoError(t, before.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	_, err := before.EndAllSessions(ctx)
	require.NoError(t, err)

	time.Sleep(2 * time.Millisecond)
	after := appplayback.NewCommandService(pr, sr, sr, ur, pfr, sharedfakes.NewFakeEventBus())
	require.NoError(t, after.ReportProgressBySession(ctx, "s1", 60_000, false, ""))
	require.NoError(t, after.StopStream(ctx, "s1"))
	require.NoError(t, after.ReportProgressBySession(ctx, "s1", 63_000, false, ""))

	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.NotNil(t, stored.EndedAt, "a tick racing this process's StopStream must not resurrect it")
}
