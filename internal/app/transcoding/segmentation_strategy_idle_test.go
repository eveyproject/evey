package transcoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMuxIsIdle covers the idle-cancel decision (fix a): the whole-file mux is
// stopped only when nobody is watching, and never while a client may be opening
// the stream or has merely buffered ahead.
func TestMuxIsIdle(t *testing.T) {
	cache, err := NewSegmentCache(t.TempDir())
	require.NoError(t, err)
	s := NewWholeFileStrategy(cache, nil, nil, nil)

	// Within the startup grace period → never idle (a client may be opening).
	if s.muxIsIdle("h", time.Now()) {
		t.Error("within grace must not be idle")
	}

	pastGrace := time.Now().Add(-(copyIdleGrace + time.Second))

	// Past grace, no segment ever served → idle (master fetched then abandoned).
	if !s.muxIsIdle("h", pastGrace) {
		t.Error("past grace with no access must be idle")
	}

	// Past grace, a segment just served → not idle (active viewer pulling).
	cache.Touch("h")
	if s.muxIsIdle("h", pastGrace) {
		t.Error("recent access must not be idle")
	}

	// Past grace, last access older than the window → idle (viewer left, buffer
	// drained well past copyIdleWindow).
	cache.mu.Lock()
	cache.accessTimes["stale"] = time.Now().Add(-(copyIdleWindow + time.Second))
	cache.mu.Unlock()
	if !s.muxIsIdle("stale", pastGrace) {
		t.Error("access older than the window must be idle")
	}

	// A recent access within the window keeps it alive even past grace — this is
	// the buffered-ahead viewer who paused fetching but is still watching.
	cache.mu.Lock()
	cache.accessTimes["buffered"] = time.Now().Add(-(copyIdleWindow / 2))
	cache.mu.Unlock()
	if s.muxIsIdle("buffered", pastGrace) {
		t.Error("access within the window must not be idle (buffered-ahead viewer)")
	}
}
