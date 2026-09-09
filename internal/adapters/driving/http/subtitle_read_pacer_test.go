package httpapi

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/pkg/netmeter"
)

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }

// Prod, 2026-09-09: a 70 GB source read flat out for its PGS track next to the
// viewer's rafale. While the streaming meter says somebody is watching, an
// extraction's reads are held to the paced rate.
func TestMediaReadPacer_PacesWhilePlaybackIsActive(t *testing.T) {
	meter := &netmeter.Meter{}
	p := newMediaReadPacer(meter, 64<<10) // 64 KiB/s: slow enough to measure
	p.sampleInterval = time.Millisecond

	// Baseline sample, then a burst of served bytes: the box is playing.
	require.False(t, p.playbackActive(time.Now()))
	meter.Add(10 << 20)
	time.Sleep(2 * time.Millisecond)
	require.True(t, p.playbackActive(time.Now()), "10 MiB served in the window is playback")

	r := &pacedReader{ReadSeekCloser: nopCloser{bytes.NewReader(make([]byte, 256<<10))}, pacer: p, ctx: context.Background()}
	// The first burst (4 MiB deep bucket) is free; reads beyond it pay the
	// rate. Drain the bucket, then time one more read.
	p.limiter.AllowN(time.Now(), subtitleReadBurst)
	start := time.Now()
	_, err := io.ReadFull(r, make([]byte, 32<<10))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond, "32 KiB at 64 KiB/s with an empty bucket takes ~0.5 s")
}

// An idle box reads unpaced: nobody is waiting on the disk.
func TestMediaReadPacer_UnpacedWhileIdle(t *testing.T) {
	meter := &netmeter.Meter{}
	p := newMediaReadPacer(meter, 1<<10) // 1 KiB/s - would take minutes if it applied
	p.sampleInterval = time.Millisecond
	require.False(t, p.playbackActive(time.Now()))
	time.Sleep(2 * time.Millisecond)
	require.False(t, p.playbackActive(time.Now()), "nothing served: idle")

	r := &pacedReader{ReadSeekCloser: nopCloser{bytes.NewReader(make([]byte, 1<<20))}, pacer: p, ctx: context.Background()}
	start := time.Now()
	_, err := io.ReadFull(r, make([]byte, 1<<20))
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 200*time.Millisecond, "idle reads must not wait on the limiter")
}

// A cancelled extraction must not sit in the limiter: the wait ends with the
// context, and the caller sees the cancellation on its next step.
func TestMediaReadPacer_CancelledContextStopsTheWait(t *testing.T) {
	meter := &netmeter.Meter{}
	p := newMediaReadPacer(meter, 1<<10)
	p.sampleInterval = time.Millisecond
	require.False(t, p.playbackActive(time.Now()))
	meter.Add(10 << 20)
	time.Sleep(2 * time.Millisecond)
	require.True(t, p.playbackActive(time.Now()))
	p.limiter.AllowN(time.Now(), subtitleReadBurst)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	p.throttle(ctx, 1<<20)
	assert.Less(t, time.Since(start), 200*time.Millisecond)
}

// No meter, no pacer, no options: the mkvgo calls run exactly as before.
func TestMediaReadPacer_NilIsUnpacedAndOptionless(t *testing.T) {
	var p *mediaReadPacer
	assert.False(t, p.playbackActive(time.Now()))
	assert.Nil(t, p.options(context.Background()))
	p.throttle(context.Background(), 1<<30) // must not block or panic

	// A real pacer hands mkvgo an FS whose Open goes through the reader.
	opts := newMediaReadPacer(&netmeter.Meter{}, 1).options(context.Background())
	require.Len(t, opts, 1)
	require.NotNil(t, opts[0].FS)
	assert.NotNil(t, opts[0].FS.Open)
}
