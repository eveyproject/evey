package bgtasks

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStop_CancelsEveryItemInFlightAndPauses(t *testing.T) {
	r := New()
	r.RegisterControllable("intro")
	// A fan-out worker: two items running at once, both must be reached.
	ctxA, doneA := r.BeginCtx(context.Background(), "intro", "a")
	ctxB, doneB := r.BeginCtx(context.Background(), "intro", "b")
	defer doneA()
	defer doneB()
	require.NoError(t, ctxA.Err())
	require.NoError(t, ctxB.Err())

	require.True(t, r.Stop("intro"))
	require.ErrorIs(t, ctxA.Err(), context.Canceled)
	require.ErrorIs(t, ctxB.Err(), context.Canceled)
	assert.True(t, r.Paused("intro"))

	snap := r.Snapshot()
	require.Len(t, snap, 1)
	assert.True(t, snap[0].Paused)
	assert.True(t, snap[0].Controllable)
	assert.Equal(t, StateRunning, snap[0].State, "the items have not returned yet")

	doneA()
	assert.Equal(t, StateRunning, r.Snapshot()[0].State, "one item still in flight")
	doneB()
	assert.Equal(t, StateIdle, r.Snapshot()[0].State)
	assert.EqualValues(t, 2, r.Snapshot()[0].Processed)
}

func TestBeginCtx_OnPausedWorkerIsAlreadyCancelled(t *testing.T) {
	r := New()
	r.RegisterControllable("kf")
	require.True(t, r.Stop("kf"))
	// A worker that skipped the WaitResumed gate still cannot fork anything.
	ctx, done := r.BeginCtx(context.Background(), "kf", "x")
	defer done()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	assert.Equal(t, "kf", TaskFromContext(ctx), "the item context carries the task for fork attribution")

	require.True(t, r.Resume("kf"))
	ctx2, done2 := r.BeginCtx(context.Background(), "kf", "y")
	defer done2()
	assert.NoError(t, ctx2.Err())
}

func TestStop_RefusesUnknownAndNonControllable(t *testing.T) {
	r := New()
	r.Register("plain")
	assert.False(t, r.Stop("plain"), "a worker that never opted in cannot be stopped")
	assert.False(t, r.Stop("nope"))
	assert.False(t, r.Resume("plain"))
	assert.False(t, r.Paused("plain"))
	assert.False(t, r.Snapshot()[0].Controllable)
}

func TestWaitResumed_BlocksUntilResumeOrShutdown(t *testing.T) {
	r := New()
	r.pollInterval = 5 * time.Millisecond
	r.RegisterControllable("lvl")
	require.NoError(t, r.WaitResumed(context.Background(), "lvl", nil), "not paused: returns at once")

	require.True(t, r.Stop("lvl"))
	released := make(chan error, 1)
	go func() { released <- r.WaitResumed(context.Background(), "lvl", nil) }()
	select {
	case err := <-released:
		t.Fatalf("returned while paused: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	require.True(t, r.Resume("lvl"))
	select {
	case err := <-released:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Resume did not release the gate")
	}

	// The worker's own stop channel wins over the pause: shutdown must not
	// hang on an admin's Stop.
	require.True(t, r.Stop("lvl"))
	stop := make(chan struct{})
	close(stop)
	require.ErrorIs(t, r.WaitResumed(context.Background(), "lvl", stop), ErrStopped)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, r.WaitResumed(ctx, "lvl", nil), context.Canceled)
}

func TestDone_ClearsTheItemContext(t *testing.T) {
	r := New()
	r.RegisterControllable("tp")
	ctx, done := r.BeginCtx(context.Background(), "tp", "x")
	done()
	require.ErrorIs(t, ctx.Err(), context.Canceled, "the derived context must not outlive the item")
	assert.True(t, r.Snapshot()[0].StartedAt.IsZero())
	// Stop after done finds nothing to cancel and still pauses.
	require.True(t, r.Stop("tp"))
	assert.True(t, r.Paused("tp"))
}
