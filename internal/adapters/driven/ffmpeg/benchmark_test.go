package ffmpeg_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	transcoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// TestBenchmarkRunner_TestSrcFallback_ReturnsValidCap exercises the
// runner end-to-end with the synthetic testsrc input (avoids depending
// on a bundled mire file). Lightweight: 1 s per rung, runs on every
// CI box with ffmpeg installed.
func TestBenchmarkRunner_TestSrcFallback_ReturnsValidCap(t *testing.T) {
	requireFFmpeg(t)

	r := ffmpeg.NewBenchmarkRunner(ffmpeg.BenchmarkOpts{
		SamplePath:  "/nonexistent/forces/fallback.mkv",
		DurationSec: 1,   // very short — we don't need accurate FPS here, just a non-zero pass
		Margin:      0.1, // very permissive — even a slow CI box must report >= 720p
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cap, err := r.Run(ctx, transcoding.HWPreference{})
	require.NoError(t, err)

	// Floor guarantee: even on the weakest possible runner the bench
	// must NOT report 0 — that would brick playback.
	assert.GreaterOrEqual(t, cap.SoftwareMaxHeight, 720, "software cap must be at least 720p (universal floor)")
	assert.Positive(t, cap.BenchmarkDurationMs)
	assert.NotZero(t, cap.BenchmarkedAt)
}

// TestBenchmarkRunner_SingleVariant_RunsEndToEnd proves the single-rung probe
// shape (the runtime default) executes a real ffmpeg encode and returns a valid
// cap — the bench must model decode + 1 encode, not the parallel ladder, or it
// pins the HDR cap far below what an on-demand session sustains.
func TestBenchmarkRunner_SingleVariant_RunsEndToEnd(t *testing.T) {
	requireFFmpeg(t)

	r := ffmpeg.NewBenchmarkRunner(ffmpeg.BenchmarkOpts{
		SamplePath:    "/nonexistent/forces/fallback.mkv",
		DurationSec:   1,
		Margin:        0.1, // permissive — even a slow CI box clears the floor
		SingleVariant: true,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	gotCap, err := r.Run(ctx, transcoding.HWPreference{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, gotCap.SoftwareMaxHeight, 720, "single-variant bench must still floor at 720p")
	assert.NotZero(t, gotCap.BenchmarkedAt)
}

// TestBenchmarkRunner_RespectMargin proves the FPS threshold check
// short-circuits the ladder when margin is unsatisfiable.
func TestBenchmarkRunner_RespectMargin(t *testing.T) {
	requireFFmpeg(t)

	// Margin of 1000× realtime is impossible to hit, so every probe fails
	// and the runner must still return the 720p floor (never zero).
	r := ffmpeg.NewBenchmarkRunner(ffmpeg.BenchmarkOpts{
		SamplePath:  "/forces/fallback.mkv",
		DurationSec: 1,
		Margin:      1000.0,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cap, err := r.Run(ctx, transcoding.HWPreference{})
	require.NoError(t, err)
	assert.Equal(t, 720, cap.SoftwareMaxHeight, "impossible margin must still floor at 720p (not 0)")
}
