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
// The unit of capacity is the STREAM, not the ffmpeg process: slots are keyed
// by decision hash and ref-counted. One viewer whose player probes a second
// ABR rung spawns a second encoder for the SAME hash and must NOT be refused
// by its own stream's slot (verified live: with capacity 1 and per-process
// counting, a single viewer 503'd itself on an ABR switch). Those sibling
// encoders are SIGSTOPped by pauseOtherVariants anyway, so they cost no CPU.
// Distinct viewers at the same position share a hash - and share the same
// encoder and cache - so they legitimately count as one.
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
	mu    sync.Mutex
	limit int
	// refs counts live encoders per decision hash. len(refs) is the number of
	// distinct encoded STREAMS, which is what the limit caps.
	refs map[string]int
}

// NewVideoSessionLimiter creates a limiter with the given initial limit (0 =
// unlimited).
func NewVideoSessionLimiter(limit int) *VideoSessionLimiter {
	if limit < 0 {
		limit = 0
	}
	return &VideoSessionLimiter{limit: limit, refs: make(map[string]int)}
}

// TryAcquire reserves a video-encode slot for the stream identified by key
// (the decision hash), non-blocking. An encoder joining a stream that already
// holds a slot always succeeds (ref-count only) - the cap governs distinct
// streams. Returns false only when a NEW stream would exceed the limit; the
// caller must then refuse the spawn with ErrVideoCapacityExhausted. Every
// successful TryAcquire must be matched by exactly one Release(key).
func (l *VideoSessionLimiter) TryAcquire(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.refs == nil {
		l.refs = make(map[string]int)
	}
	if _, joining := l.refs[key]; !joining {
		if l.limit > 0 && len(l.refs) >= l.limit {
			return false
		}
	}
	l.refs[key]++
	return true
}

// Release gives back the reference key took in TryAcquire; the stream's slot
// frees when its last encoder is gone. A stray Release for an unknown key is a
// no-op rather than a corrupting decrement, so a bookkeeping bug degrades to
// "reports slightly more headroom than exists" instead of poisoning the count.
func (l *VideoSessionLimiter) Release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, held := l.refs[key]
	if !held {
		return
	}
	if n <= 1 {
		delete(l.refs, key)
		return
	}
	l.refs[key] = n - 1
}

// Active reports how many distinct encoded streams currently hold a slot.
func (l *VideoSessionLimiter) Active() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.refs)
}

// Limit reports the current cap (0 = unlimited).
func (l *VideoSessionLimiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// SetLimit changes the cap live. Negative values clamp to 0 (unlimited). Only
// affects future TryAcquire calls for NEW streams - never kills a stream
// already counted in Active().
func (l *VideoSessionLimiter) SetLimit(n int) {
	if n < 0 {
		n = 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = n
}
