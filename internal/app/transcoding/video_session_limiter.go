package transcoding

import (
	"errors"
	"sync"
)

// ErrVideoCapacityExhausted is returned by a StreamProducer when a session
// that would ENCODE video (never a video-copy/remux rung, which never
// touches this limiter) cannot start because the shared VideoSessionLimiter
// is already at its configured limit. The HTTP handler maps it to a 503 with
// Retry-After, mirroring ErrCapacityExceeded (the orchestrator's older,
// unrelated global-rafale cap).
var ErrVideoCapacityExhausted = errors.New("transcoding: max video transcode sessions reached")

// VideoSessionLimiter is a shared, thread-safe cap on the number of
// concurrently-encoding VIDEO transcode sessions across BOTH StreamProducer
// implementations (the software path's transcode instance and the hardware
// SegmentOrchestrator) - never the software producer's dedicated COPY
// instance, which copies the video stream and must never be gated here (see
// SoftwareProducer.copyPlanner).
//
// Limit()==0 (the zero value) means UNLIMITED: TryAcquire always succeeds.
// This is the default, so a caller that never wires a limiter (or wires one
// with limit 0) observes byte-identical behaviour to before this feature
// existed. Active() is tracked even while unlimited, so a limit applied
// later via SetLimit immediately sees the true in-flight count instead of
// starting from zero.
//
// SetLimit takes effect on the NEXT TryAcquire only - it never kills an
// already-running session, matching the "admin can only reduce future
// capacity" contract.
type VideoSessionLimiter struct {
	mu     sync.Mutex
	limit  int
	active int
}

// NewVideoSessionLimiter creates a limiter with the given initial limit (0 =
// unlimited).
func NewVideoSessionLimiter(limit int) *VideoSessionLimiter {
	if limit < 0 {
		limit = 0
	}
	return &VideoSessionLimiter{limit: limit}
}

// TryAcquire reserves one video-encode slot, non-blocking. Returns true when
// a slot was taken (or the limiter is unlimited); false when the limiter is
// at capacity and the caller must refuse the spawn with
// ErrVideoCapacityExhausted. Every successful TryAcquire must be matched by
// exactly one Release.
func (l *VideoSessionLimiter) TryAcquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit > 0 && l.active >= l.limit {
		return false
	}
	l.active++
	return true
}

// Release frees one video-encode slot taken by a successful TryAcquire.
// Never goes negative - a stray extra Release is a no-op rather than a
// corrupting decrement, so a bug in caller bookkeeping degrades to "reports
// slightly more headroom than actually exists" rather than a poisoned
// negative count.
func (l *VideoSessionLimiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active > 0 {
		l.active--
	}
}

// Active reports how many video-encode slots are currently held.
func (l *VideoSessionLimiter) Active() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

// Limit reports the current cap (0 = unlimited).
func (l *VideoSessionLimiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// SetLimit changes the cap live. Negative values clamp to 0 (unlimited). Only
// affects future TryAcquire calls - never kills a session already counted in
// Active().
func (l *VideoSessionLimiter) SetLimit(n int) {
	if n < 0 {
		n = 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = n
}
