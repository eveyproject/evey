package httpapi

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }

func testPacer(t *testing.T, playing *atomic.Bool, bytesPerSec int) *mediaReadPacer {
	t.Helper()
	p := newMediaReadPacer(playing.Load, bytesPerSec)
	p.sampleInterval = 0 // ask every time
	return p
}

// While somebody is watching, reads are paced.
func TestMediaReadPacer_PacesWhilePlaybackIsActive(t *testing.T) {
	var playing atomic.Bool
	playing.Store(true)
	p := testPacer(t, &playing, 64<<10) // 64 KiB/s: slow enough to measure
	require.True(t, p.playbackActive(time.Now()))

	r := &pacedReader{ReadSeekCloser: nopCloser{bytes.NewReader(make([]byte, 256<<10))}, pacer: p, ctx: context.Background()}
	// Drain the free burst, then time one read.
	p.limiter.AllowN(time.Now(), subtitleReadBurst)
	start := time.Now()
	_, err := io.ReadFull(r, make([]byte, 32<<10))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond, "32 KiB at 64 KiB/s with an empty bucket takes ~0.5 s")
}

// Idle box: unpaced.
func TestMediaReadPacer_UnpacedWhileIdle(t *testing.T) {
	var playing atomic.Bool
	p := testPacer(t, &playing, 1<<10) // 1 KiB/s - would take minutes if it applied
	require.False(t, p.playbackActive(time.Now()))

	r := &pacedReader{ReadSeekCloser: nopCloser{bytes.NewReader(make([]byte, 1<<20))}, pacer: p, ctx: context.Background()}
	start := time.Now()
	_, err := io.ReadFull(r, make([]byte, 1<<20))
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 200*time.Millisecond, "idle reads must not wait on the limiter")
}

// The answer is sampled: a flip is seen after the interval, not before.
func TestMediaReadPacer_SamplesTheActivityAnswer(t *testing.T) {
	var playing atomic.Bool
	p := newMediaReadPacer(playing.Load, 1<<20)
	p.sampleInterval = time.Hour
	now := time.Now()
	require.False(t, p.playbackActive(now))
	playing.Store(true)
	assert.False(t, p.playbackActive(now.Add(time.Minute)), "inside the interval the sample stands")
	assert.True(t, p.playbackActive(now.Add(2*time.Hour)), "past it the answer is refreshed")
}

// A cancelled extraction must not sit in the limiter.
func TestMediaReadPacer_CancelledContextStopsTheWait(t *testing.T) {
	var playing atomic.Bool
	playing.Store(true)
	p := testPacer(t, &playing, 1<<10)
	p.limiter.AllowN(time.Now(), subtitleReadBurst)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	p.throttle(ctx, 1<<20)
	assert.Less(t, time.Since(start), 200*time.Millisecond)
}

// No pacer: no options, mkvgo runs as before.
func TestMediaReadPacer_NilIsUnpacedAndOptionless(t *testing.T) {
	var p *mediaReadPacer
	assert.False(t, p.playbackActive(time.Now()))
	assert.Nil(t, p.options(context.Background()))
	p.throttle(context.Background(), 1<<30) // must not block or panic

	opts := newMediaReadPacer(func() bool { return false }, 1).options(context.Background())
	require.Len(t, opts, 1)
	require.NotNil(t, opts[0].FS)
	assert.NotNil(t, opts[0].FS.Open)
}
