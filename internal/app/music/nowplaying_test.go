package music

import (
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
	assert.Empty(t, r.Heartbeat("s1", "prf-1", "trk-1", "Living Room", 5000, false))
	got := r.List()
	if assert.Len(t, got, 1) {
		assert.Equal(t, "s1", got[0].SessionID)
		assert.Equal(t, "trk-1", got[0].TrackID)
		assert.Equal(t, int64(5000), got[0].PositionMs)
		assert.Equal(t, base, got[0].StartedAt)
	}

	// A later heartbeat updates position/pause but keeps StartedAt.
	clock = base.Add(10 * time.Second)
	r.Heartbeat("s1", "prf-1", "trk-1", "Living Room", 15000, true)
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
	r.Heartbeat("s1", "prf-1", "trk-1", "TV", 0, false)

	// An unknown session can't be commanded.
	assert.False(t, r.SetCommand("nope", "pause"))
	// A live session accepts the command.
	assert.True(t, r.SetCommand("s1", "pause"))

	// The command comes back on the NEXT heartbeat, exactly once.
	assert.Equal(t, "pause", r.Heartbeat("s1", "prf-1", "trk-1", "TV", 1000, true))
	assert.Empty(t, r.Heartbeat("s1", "prf-1", "trk-1", "TV", 2000, true), "command is consumed, not repeated")
}

func TestNowPlayingRegistry_Remove(t *testing.T) {
	r := NewNowPlayingRegistry()
	r.Heartbeat("s1", "prf-1", "trk-1", "TV", 0, false)
	r.Remove("s1")
	assert.Empty(t, r.List())
	r.Remove("missing") // no-op
}
