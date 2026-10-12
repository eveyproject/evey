package transcoding

// Internal test: exercises the background-preemption core (pauseRunning /
// resumePaused) directly, without the 2s/12s ticker of the public supervisor.

import (
	"context"
	"testing"
	"time"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domtc "github.com/eveyproject/evey/internal/domain/transcoding"
	tcfakes "github.com/eveyproject/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGauge struct{ n int }

func (g *fakeGauge) Active() int { return g.n }

func newPreemptSvc(tc domtc.Transcoder, jobs domtc.TranscodeJobRepository) *Service {
	return NewService(tc, tcfakes.NewFakeHWDetector(), jobs,
		tcfakes.NewFakeTranscodeProfileRepository(), sharedfakes.NewFakeEventBus(), 0)
}

func TestBackgroundPreemption_PauseThenResume(t *testing.T) {
	ctx := context.Background()
	tc := tcfakes.NewFakeTranscoder()
	jobs := tcfakes.NewFakeTranscodeJobRepository()
	svc := newPreemptSvc(tc, jobs)
	svc.SetPlaybackGauge(&fakeGauge{n: 1})

	for _, id := range []string{"j1", "j2"} {
		require.NoError(t, svc.StartTranscode(ctx, &domtc.TranscodeJob{
			ID: id, MediaItemID: "m", InputPath: "/a.mkv", InputCodec: "h264", OutputCodec: "h265",
		}))
	}

	// A viewer's transcode starts → every running background job is paused.
	svc.pauseRunning(ctx)
	assert.ElementsMatch(t, []string{"j1", "j2"}, tc.PauseCalls, "both running jobs paused")

	// Idempotent: already-paused jobs are not signalled again.
	svc.pauseRunning(ctx)
	assert.Len(t, tc.PauseCalls, 2, "no double-pause")

	// Playback goes idle → paused jobs resume and the set clears.
	svc.resumePaused(ctx)
	assert.ElementsMatch(t, []string{"j1", "j2"}, tc.ResumeCalls)
	assert.Empty(t, svc.pausedJobs, "paused set cleared after resume")

	// A later playback burst pauses them again.
	svc.pauseRunning(ctx)
	assert.Len(t, tc.PauseCalls, 4)
}

func TestBackgroundPreemption_NoGaugeReturnsImmediately(t *testing.T) {
	svc := newPreemptSvc(tcfakes.NewFakeTranscoder(), tcfakes.NewFakeTranscodeJobRepository())
	done := make(chan struct{})
	go func() { svc.SuperviseBackgroundPreemption(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor must return immediately when no gauge is wired")
	}
}
