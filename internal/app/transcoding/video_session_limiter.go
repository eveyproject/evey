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
//
// Optionally, WithCostModel wires a VideoCostModel: admission then ALSO
// spends the new stream's fractional cost (see VideoCostModel.CostForHeight)
// against a budget (default 1.0 = one whole host), on top of the SetLimit
// counter cap - both gates must pass. Two cheap-enough streams (e.g. 1440p +
// 1080p, cost 0.961 combined) can then share the machine instead of each
// being billed as a full stream, the gap the plain counter cannot express.
// With no cost model wired (the default), admission is purely by counter -
// byte-identical to the pre-cost-model behaviour.
type VideoSessionLimiter struct {
	mu    sync.Mutex
	limit int
	// refs counts live encoders per decision hash. len(refs) is the number of
	// distinct encoded STREAMS, which is what the limit caps.
	refs map[string]int
	// costs holds the fractional cost reserved per decision hash by the
	// stream's FIRST TryAcquire (0 when no cost model is wired, or the key's
	// stream never reserved one). Read back by Release to give the exact
	// amount back to used, regardless of what CostForHeight would return NOW
	// (a live calibration change must never corrupt an in-flight reservation).
	costs map[string]float64
	// used is the sum of costs currently reserved - the running total spent
	// against budget.
	used float64
	// budget is the fraction of "one whole host" new streams may spend,
	// enforced only when costModel is non-nil. Defaults to 1.0 in
	// NewVideoSessionLimiter; SetBudget clamps non-positive input back to 1.0.
	budget float64
	// costModel is nil until WithCostModel wires one - admission is then
	// purely counter-based, matching pre-cost-model behaviour exactly.
	costModel *VideoCostModel
}

// NewVideoSessionLimiter creates a limiter with the given initial limit (0 =
// unlimited) and the default budget (1.0 = one whole host, only enforced
// once a cost model is wired via WithCostModel).
func NewVideoSessionLimiter(limit int) *VideoSessionLimiter {
	if limit < 0 {
		limit = 0
	}
	return &VideoSessionLimiter{
		limit:  limit,
		refs:   make(map[string]int),
		costs:  make(map[string]float64),
		budget: 1.0,
	}
}

// WithCostModel wires m as the limiter's cost model, chainable - call before
// the first TryAcquire. A nil m explicitly reverts to pure counter-based
// admission (byte-identical to never having called WithCostModel).
func (l *VideoSessionLimiter) WithCostModel(m *VideoCostModel) *VideoSessionLimiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.costModel = m
	return l
}

// SetBudget sets the fraction of "one whole host" new streams may spend once
// a cost model is wired. Non-positive input clamps to the default 1.0 rather
// than disabling the budget (a 0 or negative budget would refuse every new
// stream, which is never the intent of a misconfigured value).
func (l *VideoSessionLimiter) SetBudget(b float64) {
	if b <= 0 {
		b = 1.0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.budget = b
}

// Budget reports the current admission budget (1.0 default).
func (l *VideoSessionLimiter) Budget() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.budget
}

// Used reports the fractional cost currently reserved across all live
// streams. Always 0 when no cost model is wired.
func (l *VideoSessionLimiter) Used() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.used
}

// Rungs proxies the wired cost model's Rungs() - every calibrated rung's
// cost + derived max-concurrent-streams, for the admin capacity panel.
// Empty (never nil) when no cost model is wired.
func (l *VideoSessionLimiter) Rungs() []RungFit {
	l.mu.Lock()
	m := l.costModel
	l.mu.Unlock()
	return m.Rungs()
}

// budgetEpsilon absorbs float64 accumulation error across several rungs'
// costs so a combination that mathematically fits (e.g. 0.530 + 0.431 =
// 0.961) is never refused by a last-bit rounding artifact.
const budgetEpsilon = 1e-9

// TryAcquire reserves a video-encode slot for the stream identified by key
// (the decision hash), non-blocking, for a session whose tallest encoded
// rendition is topHeight. An encoder joining a stream that already holds a
// slot always succeeds (ref-count only, no additional cost) - the caps
// govern distinct streams. A NEW stream is refused when either gate fails:
// the SetLimit counter cap, or - only when a cost model is wired - the
// stream's CostForHeight(topHeight) would push Used() past Budget(). The
// caller must then refuse the spawn with ErrVideoCapacityExhausted. Every
// successful TryAcquire must be matched by exactly one Release(key).
func (l *VideoSessionLimiter) TryAcquire(key string, topHeight int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.refs == nil {
		l.refs = make(map[string]int)
	}
	if l.costs == nil {
		l.costs = make(map[string]float64)
	}
	if _, joining := l.refs[key]; joining {
		l.refs[key]++
		return true
	}

	if l.limit > 0 && len(l.refs) >= l.limit {
		return false
	}

	cost := 0.0
	if l.costModel != nil {
		cost = l.costModel.CostForHeight(topHeight)
		if l.used+cost > l.budget+budgetEpsilon {
			return false
		}
	}

	l.refs[key] = 1
	l.costs[key] = cost
	l.used += cost
	return true
}

// CanAdmit answers TryAcquire's question WITHOUT reserving anything: would a
// stream identified by key, whose top rung is topHeight, be admitted right
// now? Advisory by nature - the answer can go stale the instant it returns -
// so it is only ever used to shed EARLY (at manifest time, before a player
// commits to a stream), never to authorise. The authoritative decision stays
// in TryAcquire, at spawn.
//
// A key that already holds a slot always can: its sibling encoders join it.
func (l *VideoSessionLimiter) CanAdmit(key string, topHeight int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, joining := l.refs[key]; joining {
		return true
	}
	if l.limit > 0 && len(l.refs) >= l.limit {
		return false
	}
	if l.costModel == nil {
		return true
	}
	return l.used+l.costModel.CostForHeight(topHeight) <= l.budget+budgetEpsilon
}

// Release gives back the reference key took in TryAcquire; the stream's slot
// (and its reserved cost, if any) frees when its last encoder is gone. A
// stray Release for an unknown key is a no-op rather than a corrupting
// decrement, so a bookkeeping bug degrades to "reports slightly more
// headroom than exists" instead of poisoning the count.
func (l *VideoSessionLimiter) Release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, held := l.refs[key]
	if !held {
		return
	}
	if n <= 1 {
		delete(l.refs, key)
		if cost, ok := l.costs[key]; ok {
			l.used -= cost
			if l.used < 0 {
				l.used = 0
			}
			delete(l.costs, key)
		}
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

// Holds reports whether key currently holds at least one slot. Read-only, for
// the admission shaper: a ceiling pinned to a still-held stream must not move.
func (l *VideoSessionLimiter) Holds(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refs[key] > 0
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
