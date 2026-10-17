package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/procfork"
	"github.com/eveyproject/evey/internal/pkg/reduceddl"
)

// Each fork site books a memory admission in procfork.Command and hands it
// back in Finish. A path that fails between the two, an ffmpeg that cannot
// start, must still reach Finish, or the booking outlives the process.
//
// A start fails before any exec when the context is already over, whatever
// binary or wrapper (prlimit, nice) sits in front; a missing binary only fails
// at start when nothing wraps it. These tests run sequentially on purpose: a
// parallel test taking its own booking meanwhile would move the figure. The
// transcoder's job runs on a context detached from its caller, so its start
// can only fail on a fork or pipe error, which no test can provoke on demand;
// its path carries the same Finish and is covered by TestMain's final check.

func doneContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestRunMeasured_FailedStartHandsTheBookingBack(t *testing.T) {
	// The session runs unwrapped (no nice, no cap): a missing binary fails at
	// start, as it does with an expired context.
	missing := filepath.Join(t.TempDir(), "ffmpeg-that-does-not-exist")
	before := procfork.Reserved()
	m := runMeasured(context.Background(), missing, []string{"-f", "lavfi", "-i", "testsrc=duration=1"}, nil, time.Second)
	require.NotEmpty(t, m.err, "a missing binary must be reported")
	assert.Equal(t, before, procfork.Reserved(), "the session's admission must be handed back")

	before = procfork.Reserved()
	m = runMeasured(doneContext(), "ffmpeg", []string{"-f", "lavfi", "-i", "testsrc=duration=1"}, nil, time.Second)
	require.NotEmpty(t, m.err, "a context already over must be reported")
	assert.Equal(t, before, procfork.Reserved(), "the session's admission must be handed back")
}

func TestTranscodeCompat_FailedStartHandsTheBookingBack(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	require.NoError(t, os.WriteFile(src, []byte("not a real file"), 0o600))
	before := procfork.Reserved()
	err := NewCompatTranscoder("ffmpeg").TranscodeCompat(doneContext(), src, filepath.Join(dir, "dst.mp4"), 1000, nil)
	require.Error(t, err)
	t.Logf("err: %v", err)
	assert.Equal(t, before, procfork.Reserved(), "the compat transcode's admission must be handed back")
}

func TestTranscodeReducedSlices_FailedStartHandsTheBookingBack(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	require.NoError(t, os.WriteFile(src, []byte("not a real file"), 0o600))
	before := procfork.Reserved()
	err := NewReducedTranscoder("ffmpeg").TranscodeReducedSlices(doneContext(), src, filepath.Join(dir, "job"),
		reduceddl.Spec{}, transcoding.PicturePlan{}, 1000, 0, nil)
	require.Error(t, err)
	t.Logf("err: %v", err)
	assert.Equal(t, before, procfork.Reserved(), "the reduced transcode's admission must be handed back")
}

func TestChromaprintCompute_FailedStartHandsTheBookingBack(t *testing.T) {
	before := procfork.Reserved()
	_, err := NewChromaprinter(filepath.Join(t.TempDir(), "fpcalc")).Compute(doneContext(), "/input/video.mkv", 0, 10)
	require.Error(t, err)
	t.Logf("err: %v", err)
	assert.Equal(t, before, procfork.Reserved(), "the decoder's admission must be handed back")
}
