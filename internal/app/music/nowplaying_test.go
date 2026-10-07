package music

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNowPlayingRegistry_HeartbeatListAndExpiry(t *testing.T) {
	r := NewNowPlayingRegistry()
	base := time.Unix(1000, 0)
	clock := base
	r.now = func() time.Time { return clock }

	// First heartbeat registers the session, no pending command.
	assert.Empty(t, r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "Living Room", 5000, false))
	got := r.List()
	if assert.Len(t, got, 1) {
		assert.Equal(t, "s1", got[0].SessionID)
		assert.Equal(t, "trk-1", got[0].TrackID)
		assert.Equal(t, int64(5000), got[0].PositionMs)
		assert.Equal(t, base, got[0].StartedAt)
	}

	// A later heartbeat updates position/pause but keeps StartedAt.
	clock = base.Add(10 * time.Second)
	r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "Living Room", 15000, true)
	got = r.List()
	if assert.Len(t, got, 1) {
		assert.Equal(t, int64(15000), got[0].PositionMs)
		assert.True(t, got[0].IsPaused)
		assert.Equal(t, base, got[0].StartedAt, "StartedAt is stable across heartbeats")
	}

	// Past the TTL with no heartbeat, the session is evicted.
	clock = clock.Add(nowPlayingTTL + time.Second)
	assert.Empty(t, r.List())
}

func TestNowPlayingRegistry_RemoteCommandRoundTrip(t *testing.T) {
	r := NewNowPlayingRegistry()
	r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false)

	// An unknown session can't be commanded.
	assert.False(t, r.SetCommand("nope", "pause"))
	// A live session accepts the command.
	assert.True(t, r.SetCommand("s1", "pause"))

	// The command comes back on the NEXT heartbeat, exactly once.
	assert.Equal(t, "pause", r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 1000, true))
	assert.Empty(t, r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 2000, true), "command is consumed, not repeated")
}

func TestNowPlayingRegistry_Remove(t *testing.T) {
	r := NewNowPlayingRegistry()
	r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false)
	r.Remove("s1")
	assert.Empty(t, r.List())
	r.Remove("missing") // no-op
}

// A player parked in WaitCommand gets the admin's command the moment it is
// queued, not on its next heartbeat.
func TestNowPlayingRegistry_WaitCommand_WakesOnCommand(t *testing.T) {
	r := NewNowPlayingRegistry()
	r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false)

	got := make(chan string, 1)
	go func() {
		cmd, ok := r.WaitCommand(context.Background(), "s1", "prf-1")
		assert.True(t, ok)
		got <- cmd
	}()
	assertBlocked(t, got)
	assert.True(t, r.SetCommand("s1", "pause"))

	select {
	case cmd := <-got:
		assert.Equal(t, "pause", cmd)
	case <-time.After(2 * time.Second):
		t.Fatal("WaitCommand did not wake on SetCommand")
	}
	assert.Empty(t, r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, true), "the waiter consumed it")
}

func TestNowPlayingRegistry_WaitCommand_PendingReturnsAtOnce(t *testing.T) {
	r := NewNowPlayingRegistry()
	r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false)
	r.SetCommand("s1", "stop")
	cmd, ok := r.WaitCommand(context.Background(), "s1", "prf-1")
	assert.True(t, ok)
	assert.Equal(t, "stop", cmd)
}

// Another profile cannot drain someone else's commands, and an unknown session
// has nothing to wait on.
func TestNowPlayingRegistry_WaitCommand_RefusesForeignOrUnknown(t *testing.T) {
	r := NewNowPlayingRegistry()
	r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false)
	r.SetCommand("s1", "pause")

	_, ok := r.WaitCommand(context.Background(), "s1", "prf-2")
	assert.False(t, ok)
	_, ok = r.WaitCommand(context.Background(), "nope", "prf-1")
	assert.False(t, ok)
	assert.Equal(t, "pause", r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false), "still queued for its owner")
}

func TestNowPlayingRegistry_WaitCommand_ReleasedByContextRemoveAndClose(t *testing.T) {
	r := NewNowPlayingRegistry()
	r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	cmd, ok := r.WaitCommand(ctx, "s1", "prf-1")
	assert.True(t, ok)
	assert.Empty(t, cmd)

	// Ordered on purpose: once the registry is closed, a fresh WaitCommand is
	// refused at once (asserted below), so "close" must be the last release -
	// a map here ran the two in random order and failed one run in eight.
	releases := []struct {
		name    string
		release func()
	}{
		{"remove", func() { r.Remove("s1") }},
		{"close", r.Close},
	}
	for _, step := range releases {
		name, release := step.name, step.release
		r.Heartbeat("s1", "prf-1", "Alice", "trk-1", "TV", 0, false)
		done := make(chan struct{})
		go func() {
			defer close(done)
			cmd, _ := r.WaitCommand(context.Background(), "s1", "prf-1")
			assert.Empty(t, cmd)
		}()
		assertBlocked(t, done)
		release()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not release the waiter", name)
		}
	}
	_, ok = r.WaitCommand(context.Background(), "s1", "prf-1")
	assert.False(t, ok, "a closed registry refuses new waits")
}

// assertBlocked proves the waiter is parked: WaitCommand only blocks in its
// select, so a goroutine that has not returned yet is waiting there.
func assertBlocked[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("WaitCommand returned before anything released it")
	case <-time.After(50 * time.Millisecond):
	}
}
