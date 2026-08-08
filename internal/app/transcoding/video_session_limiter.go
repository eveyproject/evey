package transcoding

import (
	"errors"
	"slices"
	"sync"

	"github.com/evey-project/evey/internal/domain/transcoding"
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
// spends the new stream's fractional cost (see VideoCostModel.CostFor)
// against a PER-POOL budget (default 1.0 = one whole pool), on top of the
// SetLimit counter cap - both gates must pass. A stream's pool is derived
// from its LadderClass (LadderClass.Pool): every software ladder
// (plain/HDR/AV1) shares PoolCPU, "hw" gets its own PoolGPU, so a GPU encode
// (NVENC/QSV/VAAPI - negligible CPU cost) never competes with software
// encoders for the same budget and vice versa. Two cheap-enough streams in
// the SAME pool (e.g. 1440p + 1080p software, cost 0.961 combined) can share
// it instead of each being billed as a full stream, the gap the plain
// counter cannot express. With no cost model wired (the default), admission
// is purely by counter - byte-identical to the pre-cost-model behaviour, and
// to the pre-pool single-budget behaviour on a box with no hw bench samples
// (CostFor falls back to the software table for any class it has no
// evidence for).
type VideoSessionLimiter struct {
	mu    sync.Mutex
	limit int
	// refs counts live encoders per decision hash. len(refs) is the number of
	// distinct encoded STREAMS, which is what the limit caps.
	refs map[string]int
	// classes holds the LadderClass reserved by each key's FIRST TryAcquire -
	// read back by Release to know which pool to credit. Only meaningful for
	// keys present in costs (i.e. a cost model was wired at reservation time).
	classes map[string]LadderClass
	// costs holds the fractional cost reserved per decision hash by the
	// stream's FIRST TryAcquire (0 when no cost model is wired, or the key's
	// stream never reserved one). Read back by Release to give the exact
	// amount back to used, regardless of what CostFor would return NOW (a
	// live calibration change must never corrupt an in-flight reservation).
	costs map[string]float64
	// used is the sum of costs currently reserved, PER POOL - the running
	// total spent against each pool's budget.
	used map[ResourcePool]float64
	// budgets is the fraction of "one whole pool" new streams may spend in
	// each pool, enforced only when costModel is non-nil. Defaults to 1.0 for
	// both PoolCPU and PoolGPU in NewVideoSessionLimiter; SetBudget clamps
	// non-positive input back to 1.0 for the pool it targets.
	budgets map[ResourcePool]float64
	// costModel is nil until WithCostModel wires one - admission is then
	// purely counter-based, matching pre-cost-model behaviour exactly.
	costModel *VideoCostModel
}

// NewVideoSessionLimiter creates a limiter with the given initial limit (0 =
// unlimited) and the default budget (1.0 = one whole pool, for both PoolCPU
// and PoolGPU, only enforced once a cost model is wired via WithCostModel).
func NewVideoSessionLimiter(limit int) *VideoSessionLimiter {
	if limit < 0 {
		limit = 0
	}
	return &VideoSessionLimiter{
		limit:   limit,
		refs:    make(map[string]int),
		classes: make(map[string]LadderClass),
		costs:   make(map[string]float64),
		used:    make(map[ResourcePool]float64),
		budgets: map[ResourcePool]float64{PoolCPU: 1.0, PoolGPU: 1.0},
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

// SetBudget sets the fraction of "one whole pool" new streams may spend in
// pool once a cost model is wired. Non-positive input clamps to the default
// 1.0 rather than disabling the budget (a 0 or negative budget would refuse
// every new stream in that pool, which is never the intent of a
// misconfigured value).
func (l *VideoSessionLimiter) SetBudget(pool ResourcePool, b float64) {
	if b <= 0 {
		b = 1.0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.budgets == nil {
		l.budgets = make(map[ResourcePool]float64)
	}
	l.budgets[pool] = b
}

// budgetFor returns pool's budget (1.0 when never explicitly set). Must be
// called with l.mu held.
func (l *VideoSessionLimiter) budgetFor(pool ResourcePool) float64 {
	if l.budgets == nil {
		return 1.0
	}
	if b, ok := l.budgets[pool]; ok {
		return b
	}
	return 1.0
}

// usedFor returns pool's currently reserved cost. Must be called with l.mu
// held.
func (l *VideoSessionLimiter) usedFor(pool ResourcePool) float64 {
	if l.used == nil {
		return 0
	}
	return l.used[pool]
}

// addUsed adjusts pool's reserved cost by delta (positive on reservation,
// negative on release), clamped so it never goes negative. Must be called
// with l.mu held.
func (l *VideoSessionLimiter) addUsed(pool ResourcePool, delta float64) {
	if l.used == nil {
		l.used = make(map[ResourcePool]float64)
	}
	l.used[pool] += delta
	if l.used[pool] < 0 {
		l.used[pool] = 0
	}
}

// Budget reports the sum of every pool's admission budget (PoolCPU +
// PoolGPU). Kept exact-signature zero-arg for back-compat with callers
// written before pools existed - e.g. the admin panel's aggregate "budget
// used/total" view (grpcapi.videoSessionActiveReporter) - which observe the
// same number they would have on a single-pool limiter, and which cannot be
// widened to take a pool argument without breaking that interface. See
// BudgetFor for the per-pool figure.
func (l *VideoSessionLimiter) Budget() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.budgetFor(PoolCPU) + l.budgetFor(PoolGPU)
}

// Used reports the sum of every pool's currently reserved cost. Kept
// exact-signature zero-arg for the same back-compat reason as Budget. See
// UsedFor for the per-pool figure. Always 0 when no cost model is wired.
func (l *VideoSessionLimiter) Used() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	var sum float64
	for _, v := range l.used {
		sum += v
	}
	return sum
}

// BudgetFor reports pool's admission budget alone (1.0 by default).
func (l *VideoSessionLimiter) BudgetFor(pool ResourcePool) float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.budgetFor(pool)
}

// UsedFor reports pool's currently reserved cost alone. Always 0 when no
// cost model is wired.
func (l *VideoSessionLimiter) UsedFor(pool ResourcePool) float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.usedFor(pool)
}

// Rungs proxies the wired cost model's Rungs() (the software ladder) - every
// calibrated software rung's cost + derived max-concurrent-streams, for the
// admin capacity panel. Empty (never nil) when no cost model is wired.
func (l *VideoSessionLimiter) Rungs() []RungFit {
	l.mu.Lock()
	m := l.costModel
	l.mu.Unlock()
	return m.Rungs()
}

// RungsFor proxies the wired cost model's RungsFor(class) - class's
// calibrated rungs, for the admin panel's per-pool breakdown (see
// VideoCostModel.RungsFor). Empty (never nil) when no cost model is wired or
// class has no table.
func (l *VideoSessionLimiter) RungsFor(class LadderClass) []RungFit {
	l.mu.Lock()
	m := l.costModel
	l.mu.Unlock()
	return m.RungsFor(class)
}

// Classes proxies the wired cost model's Classes() - which ladder classes
// have usable calibration evidence, so the admin panel knows which pools to
// render (see VideoCostModel.Classes). nil when no cost model is wired.
func (l *VideoSessionLimiter) Classes() []LadderClass {
	l.mu.Lock()
	m := l.costModel
	l.mu.Unlock()
	return m.Classes()
}

// budgetEpsilon absorbs float64 accumulation error across several rungs'
// costs so a combination that mathematically fits (e.g. 0.530 + 0.431 =
// 0.961) is never refused by a last-bit rounding artifact.
const budgetEpsilon = 1e-9

// TryAcquire reserves a video-encode slot for the stream identified by key
// (the decision hash), non-blocking, for a session in ladder class class
// whose tallest encoded rendition is topHeight. An encoder joining a stream
// that already holds a slot always succeeds (ref-count only, no additional
// cost) - the caps govern distinct streams. A NEW stream is refused when
// either gate fails: the SetLimit counter cap (GLOBAL, across every class),
// or - only when a cost model is wired - class.Pool()'s budget: the stream's
// CostFor(class, topHeight) would push that pool's Used() past its Budget().
// The caller must then refuse the spawn with ErrVideoCapacityExhausted.
// Every successful TryAcquire must be matched by exactly one Release(key).
func (l *VideoSessionLimiter) TryAcquire(key string, class LadderClass, topHeight int) bool {
	return l.TryAcquireFor(key, class, topHeight, transcoding.SourceClass{})
}

// TryAcquireFor is TryAcquire with the session's DECODE workload named, so the
// cost model prices what this stream costs rather than an average one at that
// rung. A zero src prices as TryAcquire always did.
func (l *VideoSessionLimiter) TryAcquireFor(key string, class LadderClass, topHeight int, src transcoding.SourceClass) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.refs == nil {
		l.refs = make(map[string]int)
	}
	if l.costs == nil {
		l.costs = make(map[string]float64)
	}
	if l.classes == nil {
		l.classes = make(map[string]LadderClass)
	}
	if _, joining := l.refs[key]; joining {
		l.refs[key]++
		return true
	}

	if l.limit > 0 && len(l.refs) >= l.limit {
		return false
	}

	pool := class.Pool()
	cost := 0.0
	if l.costModel != nil {
		cost = l.costModel.CostForSource(class, topHeight, src)
		if l.usedFor(pool)+cost > l.budgetFor(pool)+budgetEpsilon {
			return false
		}
	}

	l.refs[key] = 1
	l.classes[key] = class
	l.costs[key] = cost
	l.addUsed(pool, cost)
	return true
}

// CanAdmit answers TryAcquire's question WITHOUT reserving anything: would a
// stream in ladder class class, identified by key, whose top rung is
// topHeight, be admitted right now? Advisory by nature - the answer can go
// stale the instant it returns - so it is only ever used to shed EARLY (at
// manifest time, before a player commits to a stream), never to authorise.
// The authoritative decision stays in TryAcquire, at spawn.
//
// A key that already holds a slot always can: its sibling encoders join it.
func (l *VideoSessionLimiter) CanAdmit(key string, class LadderClass, topHeight int) bool {
	return l.CanAdmitFor(key, class, topHeight, transcoding.SourceClass{})
}

// CanAdmitFor is CanAdmit with the decode workload named. The shed probe must
// price like the spawn, or a viewer is turned away for a cost nothing charges.
func (l *VideoSessionLimiter) CanAdmitFor(key string, class LadderClass, topHeight int, src transcoding.SourceClass) bool {
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
	pool := class.Pool()
	return l.usedFor(pool)+l.costModel.CostForSource(class, topHeight, src) <= l.budgetFor(pool)+budgetEpsilon
}

// Release gives back the reference key took in TryAcquire; the stream's slot
// (and its reserved cost, if any, credited back to the POOL it was reserved
// from) frees when its last encoder is gone. A stray Release for an unknown
// key is a no-op rather than a corrupting decrement, so a bookkeeping bug
// degrades to "reports slightly more headroom than exists" instead of
// poisoning the count.
func (l *VideoSessionLimiter) Release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, held := l.refs[key]
	if !held {
		return
	}
	if n <= 1 {
		delete(l.refs, key)
		class := l.classes[key]
		if cost, ok := l.costs[key]; ok {
			l.addUsed(class.Pool(), -cost)
			delete(l.costs, key)
		}
		delete(l.classes, key)
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

// AutoCountCapFor is how many streams pool's CHEAPEST rung fits in its budget
// (cheapest by COST, not by height - hw costs are not monotonic in height).
// 0 when the pool has no calibration evidence.
func (l *VideoSessionLimiter) AutoCountCapFor(pool ResourcePool) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.autoCountCapFor(pool)
}

// autoCountCapFor is AutoCountCapFor with l.mu already held.
//
// The cheapest stream is taken across rungs AND source classes, not just rungs.
// This is a BELT, not the gate: the per-pool budget already prices every stream
// for what it actually decodes, so sizing the counter on the dearest source
// would refuse streams the budget has room for. Seen live on 2026-08-08: the
// budget allowed five 1080p H.264 sessions while the counter, built on the 4K
// HEVC bench source, stopped at one - the counter silently undid the whole
// point of pricing by source.
func (l *VideoSessionLimiter) autoCountCapFor(pool ResourcePool) int {
	if l.costModel == nil {
		return 0
	}
	class := LadderClassSoftware
	if pool == PoolGPU {
		class = LadderClassHW
		if !slices.Contains(l.costModel.Classes(), LadderClassHW) {
			return 0
		}
	}
	cheapest := 0.0
	for _, r := range l.costModel.RungsFor(class) {
		for _, c := range append([]float64{r.Cost}, sourceFitCosts(r.BySource)...) {
			if c > 0 && (cheapest == 0 || c < cheapest) {
				cheapest = c
			}
		}
	}
	if cheapest <= 0 {
		return 0
	}
	return int(l.budgetFor(pool) / cheapest)
}

// sourceFitCosts projects the per-source costs of one rung.
func sourceFitCosts(fits []SourceFit) []float64 {
	out := make([]float64, 0, len(fits))
	for _, f := range fits {
		out = append(out, f.Cost)
	}
	return out
}

// AutoCountCap is the count cap this host's own calibration derives: the sum
// across pools of AutoCountCapFor. 0 means "no evidence to derive one from".
//
// It lives HERE, on the limiter that owns both the cost model and the budgets,
// because it used to be computed inline at the wiring site while the admin API
// derived a DIFFERENT number from the tallest bench rung. The panel then showed
// an "effective limit" that was not the one being enforced.
func (l *VideoSessionLimiter) AutoCountCap() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.autoCountCapFor(PoolCPU) + l.autoCountCapFor(PoolGPU)
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
