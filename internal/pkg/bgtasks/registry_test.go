package bgtasks

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_BeginDoneAndSnapshot(t *testing.T) {
	r := New()
	r.Register("worker-a")

	// Registered but never run → idle, 0 processed, in the snapshot.
	snap := r.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "worker-a", snap[0].Name)
	assert.Equal(t, StateIdle, snap[0].State)
	assert.EqualValues(t, 0, snap[0].Processed)

	// While running it reports the detail.
	done := r.Begin("worker-a", "media-42")
	snap = r.Snapshot()
	assert.Equal(t, StateRunning, snap[0].State)
	assert.Equal(t, "media-42", snap[0].Detail)
	assert.False(t, snap[0].LastActiveAt.IsZero())

	// done() flips to idle, clears detail, bumps processed.
	done()
	snap = r.Snapshot()
	assert.Equal(t, StateIdle, snap[0].State)
	assert.Empty(t, snap[0].Detail)
	assert.EqualValues(t, 1, snap[0].Processed)

	// done() is idempotent - a double call must not double-count.
	done()
	assert.EqualValues(t, 1, r.Snapshot()[0].Processed)
}

func TestRegistry_BeginAutoRegisters_SortedSnapshot(t *testing.T) {
	r := New()
	r.Begin("zeta", "")() // never Register'd → Begin creates it
	r.Begin("alpha", "")()

	snap := r.Snapshot()
	require.Len(t, snap, 2)
	assert.Equal(t, "alpha", snap[0].Name, "snapshot must be name-sorted")
	assert.Equal(t, "zeta", snap[1].Name)
}

func TestQueue_TracksTheBatchUntilItDrains(t *testing.T) {
	r := New()
	r.Queue("scan", 3)
	for i := range 3 {
		_, done := r.BeginCtx(context.Background(), "scan", "f")
		snap := r.Snapshot()[0]
		assert.EqualValues(t, 3, snap.BatchTotal)
		assert.EqualValues(t, i, snap.BatchDone)
		done()
	}
	snap := r.Snapshot()[0]
	assert.Zero(t, snap.BatchTotal, "a drained batch is over")
	assert.Zero(t, snap.BatchDone)

	// An item announced then dropped must not stick to the next batch.
	r.Queue("scan", 2)
	_, done := r.BeginCtx(context.Background(), "scan", "f")
	done()
	r.mu.Lock()
	r.m["scan"].lastActive = time.Now().Add(-2 * batchStale)
	r.mu.Unlock()
	r.Queue("scan", 1)
	snap = r.Snapshot()[0]
	assert.EqualValues(t, 1, snap.BatchTotal)
	assert.Zero(t, snap.BatchDone)
}
