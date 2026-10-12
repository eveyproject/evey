package transcoding

import (
	"errors"
	"math"
	"slices"
	"sync"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// ErrVideoCapacityExhausted is returned by a StreamProducer when a session
// that would ENCODE video (never a video-copy/remux rung, which never
// touches this limiter) cannot start because the shared VideoSessionLimiter
// is already at its configured limit. The HTTP handler maps it to a 503 with
// Retry-After, mirroring ErrCapacityExceeded (the orchestrator's older,
// unrelated global-rafale cap).
var ErrVideoCapacityExhausted = errors.New("transcoding: max video transcode sessions reached")

// VideoSessionLimiter caps the concurrently-ENCODING video sessions across both
// producers (never the copy instance, which encodes no video). The unit is the
// STREAM (decision hash, ref-counted), not the ffmpeg process: a viewer probing
// a second ABR rung must not be refused by its own stream's slot (seen live at
// capacity 1). Limit()==0 = unlimited; SetLimit only affects the NEXT TryAcquire.
// With a VideoCostModel wired, admission also spends the stream's fractional
// cost against a per-pool budget (software ladders share PoolCPU, hw gets
// PoolGPU) so two cheap streams can share a pool the plain counter would refuse.
type VideoSessionLimiter struct {
	mu    sync.Mutex
	limit int
	// refs counts live encoders per decision hash. len(refs) is the number of
	// distinct encoded STREAMS, which is what the limit caps.
	refs map[string]int
	// reservations holds the per-pool charges reserved by each key's FIRST
	// TryAcquire, read back by Release to credit the EXACT amounts to the same
	// pools - regardless of what the model would return now (a live
	// recalibration must never corrupt an in-flight reservation). A route with
	// evidence charges two pools (accelerator + CPU); a table-priced session
	// charges one.
	reservations map[string][]poolCharge
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
	// routeModel prices a session on the per-route single-session evidence
	// (RouteCostModel). Consulted FIRST for a route it measured; costModel is
	// the fallback for a class/rung it did not. nil = no route evidence.
	routeModel *RouteCostModel
}

// poolCharge is one pool's reserved cost for a stream.
type poolCharge struct {
	pool ResourcePool
	cost float64
}

// NewVideoSessionLimiter creates a limiter with the given initial limit (0 =
// unlimited) and the default budget (1.0 = one whole pool, for both PoolCPU
// and PoolGPU, only enforced once a cost model is wired via WithCostModel).
func NewVideoSessionLimiter(limit int) *VideoSessionLimiter {
	if limit < 0 {
		limit = 0
	}
	return &VideoSessionLimiter{
		limit:        limit,
		refs:         make(map[string]int),
		reservations: make(map[string][]poolCharge),
		used:         make(map[ResourcePool]float64),
		budgets:      map[ResourcePool]float64{PoolCPU: 1.0, PoolGPU: 1.0},
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

// WithRouteModel wires the per-route cost model (RouteCostModel), consulted
// before the table for a route it measured. nil reverts to table pricing.
func (l *VideoSessionLimiter) WithRouteModel(m *RouteCostModel) *VideoSessionLimiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.routeModel = m
	return l
}

// chargesFor returns the per-pool charges a NEW stream of (class, topHeight,
// src, route) reserves. The route model (measured, both pools) wins when it
// has evidence for the route; otherwise the table (one pool, the class's).
// Must be called with l.mu held.
func (l *VideoSessionLimiter) chargesFor(class LadderClass, topHeight int, src transcoding.SourceClass, route transcoding.RouteCostKey, sourceFPS float64) ([]poolCharge, bool) {
	// A host WITH route evidence is priced ONLY on it. A NAMED route that
	// evidence does not cover is not silently re-priced on the generic
	// (h264-shaped) table - that is exactly how x265 / hevc_vaapi / bridge /
	// decoupled got charged as if they were plain h264. It is REFUSED here
	// instead, so the resolver spills it to the measured software route (or the
	// request retries / is refused) rather than being admitted on a fiction.
	// A ZERO route is not "uncovered", it is "unnamed" (TryAcquire / CanAdmit):
	// the documented contract is the table, as before route evidence existed.
	// Refusing it turned every master-playlist shed probe away as soon as a
	// host had route evidence (prod 2026-09-13 21:53: "host at video capacity"
	// on an idle server, every title).
	if l.routeModel != nil && route != (transcoding.RouteCostKey{}) {
		rc, ok := l.routeModel.Cost(route, topHeight, sourceFPS)
		if !ok {
			return nil, false
		}
		var charges []poolCharge
		if rc.Accel > 0 {
			charges = append(charges, poolCharge{class.Pool(), rc.Accel})
		}
		if rc.CPU > 0 {
			charges = append(charges, poolCharge{PoolCPU, rc.CPU})
		}
		return charges, true
	}
	// No route evidence at all (a capability benched before it existed): the
	// table is the only signal there is, priced as before.
	if l.costModel != nil {
		return []poolCharge{{class.Pool(), l.costModel.CostForSource(class, topHeight, src)}}, true
	}
	return nil, true
}

// fits reports whether every charge stays inside its pool's budget. Held.
func (l *VideoSessionLimiter) fits(charges []poolCharge) bool {
	for _, c := range charges {
		if l.usedFor(c.pool)+c.cost > l.budgetFor(c.pool)+budgetEpsilon {
			return false
		}
	}
	return true
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

// Rungs is RungsFor(LadderClassSoftware), for the admin capacity panel.
func (l *VideoSessionLimiter) Rungs() []RungFit {
	return l.RungsFor(LadderClassSoftware)
}

// RungsFor is class's calibrated rungs for the admin panel's per-pool
// breakdown. The rung heights come from the cost model's table, but on a host
// WITH route evidence the cost and the stream counts come from chargesFor on
// the class's representative route - the very pricing admission applies - so
// the panel can never promise streams admission will refuse (the table said ~25
// where admission fit ~4). A rung the route evidence does not cover is omitted:
// admission refuses it. Empty (never nil) when no cost model is wired or class
// has no table.
func (l *VideoSessionLimiter) RungsFor(class LadderClass) []RungFit {
	l.mu.Lock()
	defer l.mu.Unlock()
	rungs := l.costModel.RungsFor(class)
	if l.routeModel == nil {
		return rungs
	}
	route, ok := representativeRoute(l.routeModel, class)
	out := make([]RungFit, 0, len(rungs))
	if !ok {
		return out
	}
	for _, r := range rungs {
		fit, ok := l.routeRungFit(class, route, r)
		if !ok {
			continue
		}
		out = append(out, fit)
	}
	return out
}

// representativeRoute is the measured route a session of class most commonly
// takes: the plain-SDR H.264 encode (the hardware backend's, for the hw class;
// libx264 / its HDR tone-map / libsvtav1 for the software classes). ok=false
// when the evidence holds no such route.
func representativeRoute(m *RouteCostModel, class LadderClass) (transcoding.RouteCostKey, bool) {
	var want func(k transcoding.RouteCostKey) bool
	switch class {
	case LadderClassHW:
		want = func(k transcoding.RouteCostKey) bool {
			return k.Backend != "" && k.Route == transcoding.RouteCostSDR && k.Encoder == transcoding.RouteEncoderName("h264", k.Backend)
		}
	case LadderClassSoftwareHDR:
		want = func(k transcoding.RouteCostKey) bool {
			return k.Backend == "" && k.Route == transcoding.RouteCostSoftwareHDR
		}
	case LadderClassSoftwareAV1:
		want = func(k transcoding.RouteCostKey) bool { return k.Backend == "" && k.Encoder == "libsvtav1" }
	default:
		want = func(k transcoding.RouteCostKey) bool {
			return k.Backend == "" && k.Route == transcoding.RouteCostSDR && k.Encoder == "libx264"
		}
	}
	for _, k := range m.Keys() {
		if want(k) {
			return k, true
		}
	}
	return transcoding.RouteCostKey{}, false
}

// routeRungFit re-prices one table rung on route through chargesFor. Held.
func (l *VideoSessionLimiter) routeRungFit(class LadderClass, route transcoding.RouteCostKey, r RungFit) (RungFit, bool) {
	charges, ok := l.chargesFor(class, r.Height, transcoding.SourceClass{}, route, 0)
	if !ok {
		return RungFit{}, false
	}
	fit := RungFit{Height: r.Height, Cost: maxChargeCost(charges), MaxStreams: l.streamsFitting(charges)}
	// Route evidence carries no decode-source dimension: every source class is
	// the same session, priced the same.
	for _, s := range r.BySource {
		fit.BySource = append(fit.BySource, SourceFit{Source: s.Source, Cost: fit.Cost, MaxStreams: fit.MaxStreams})
	}
	return fit, true
}

// streamsFitting is how many sessions priced as charges fit the pools' whole
// budgets from idle: the tighter pool decides, never their sum. Held.
func (l *VideoSessionLimiter) streamsFitting(charges []poolCharge) int {
	n := -1
	for _, c := range charges {
		if c.cost <= 0 {
			continue
		}
		k := int(math.Floor((l.budgetFor(c.pool) + budgetEpsilon) / c.cost))
		if n < 0 || k < n {
			n = k
		}
	}
	if n < 0 {
		return 1
	}
	return n
}

func maxChargeCost(charges []poolCharge) float64 {
	var m float64
	for _, c := range charges {
		m = max(m, c.cost)
	}
	return m
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

// SessionCost is what admission needs to price one session: the class (its
// pool), the tallest rung it encodes, the decode workload, the exact route and
// the source rate. Built in ONE place (SessionCostOf) from the fingerprint, so
// the spawn and the shed probe cannot price the same stream differently.
type SessionCost struct {
	Class     LadderClass
	TopHeight int
	Src       transcoding.SourceClass
	Route     transcoding.RouteCostKey
	SourceFPS float64
}

// SessionCostOf prices the session a fingerprint describes, topping out at
// topHeight (the tallest rung it will encode).
func SessionCostOf(fp DecisionFingerprint, topHeight int) SessionCost {
	return SessionCost{Class: LadderClassOf(fp), TopHeight: topHeight, Src: SourceClassOf(fp), Route: RouteCostKeyOf(fp), SourceFPS: fp.SourceFPS}
}

// TryAcquireSession reserves a slot for a NEW stream priced as c - the entry
// the producers use at spawn. See TryAcquireRoute for the contract.
func (l *VideoSessionLimiter) TryAcquireSession(key string, c SessionCost) bool {
	return l.TryAcquireRoute(key, c.Class, c.TopHeight, c.Src, c.Route, c.SourceFPS)
}

// CanAdmitSession is TryAcquireSession without the reservation - the shed
// probe's question.
func (l *VideoSessionLimiter) CanAdmitSession(key string, c SessionCost) bool {
	return l.CanAdmitRoute(key, c.Class, c.TopHeight, c.Src, c.Route, c.SourceFPS)
}

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
	return l.TryAcquireRoute(key, class, topHeight, transcoding.SourceClass{}, transcoding.RouteCostKey{}, 0)
}

// TryAcquireFor is TryAcquire with the session's DECODE workload named, so the
// cost model prices what this stream costs rather than an average one at that
// rung. A zero src prices as TryAcquire always did.
func (l *VideoSessionLimiter) TryAcquireFor(key string, class LadderClass, topHeight int, src transcoding.SourceClass) bool {
	return l.TryAcquireRoute(key, class, topHeight, src, transcoding.RouteCostKey{}, 0)
}

// TryAcquireRoute is TryAcquireFor with the resolved route (RouteCostKeyOf) and
// the source frame rate, so the route model prices the accelerator AND the CPU
// pool the session really spends. route zero / sourceFPS 0 fall back to the
// table exactly as TryAcquireFor did.
func (l *VideoSessionLimiter) TryAcquireRoute(key string, class LadderClass, topHeight int, src transcoding.SourceClass, route transcoding.RouteCostKey, sourceFPS float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.refs == nil {
		l.refs = make(map[string]int)
	}
	if l.reservations == nil {
		l.reservations = make(map[string][]poolCharge)
	}
	if _, joining := l.refs[key]; joining {
		l.refs[key]++
		return true
	}

	if l.limit > 0 && len(l.refs) >= l.limit {
		return false
	}

	charges, ok := l.chargesFor(class, topHeight, src, route, sourceFPS)
	if !ok || !l.fits(charges) {
		return false
	}

	l.refs[key] = 1
	l.reservations[key] = charges
	for _, c := range charges {
		l.addUsed(c.pool, c.cost)
	}
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
// That shortcut is keyed on the limiter's OWN namespace (fingerprint hashes).
// The admission shaper probes with a STREAM key instead, which never matches a
// ref, so for that caller the shortcut is dead and every probe prices as a new
// stream - harmless today, a trap for a caller that passes a real hash.
func (l *VideoSessionLimiter) CanAdmit(key string, class LadderClass, topHeight int) bool {
	return l.CanAdmitRoute(key, class, topHeight, transcoding.SourceClass{}, transcoding.RouteCostKey{}, 0)
}

// CanAdmitFor is CanAdmit with the decode workload named. The shed probe must
// price like the spawn, or a viewer is turned away for a cost nothing charges.
func (l *VideoSessionLimiter) CanAdmitFor(key string, class LadderClass, topHeight int, src transcoding.SourceClass) bool {
	return l.CanAdmitRoute(key, class, topHeight, src, transcoding.RouteCostKey{}, 0)
}

// CanAdmitRoute is CanAdmitFor with the route + source fps (see TryAcquireRoute).
func (l *VideoSessionLimiter) CanAdmitRoute(key string, class LadderClass, topHeight int, src transcoding.SourceClass, route transcoding.RouteCostKey, sourceFPS float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, joining := l.refs[key]; joining {
		return true
	}
	if l.limit > 0 && len(l.refs) >= l.limit {
		return false
	}
	if l.costModel == nil && l.routeModel == nil {
		return true
	}
	charges, ok := l.chargesFor(class, topHeight, src, route, sourceFPS)
	return ok && l.fits(charges)
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
		for _, c := range l.reservations[key] {
			l.addUsed(c.pool, -c.cost)
		}
		delete(l.reservations, key)
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
	// With route evidence (v7), the count belt is derived from IT, never from
	// the cumulative table: the table over-prices a GPU session (the ~2x the
	// route-bench measured), which would make this legacy belt TIGHTER than the
	// route-aware per-pool admission - the belt must never be the real ceiling.
	// The per-pool budget stays the authoritative gate; this is only the belt.
	if l.routeModel != nil {
		cpu, gpu := l.routeModel.CheapestPoolCost()
		cheapest := cpu
		if pool == PoolGPU {
			cheapest = gpu
		}
		if cheapest <= 0 {
			return 0
		}
		return int(l.budgetFor(pool) / cheapest)
	}
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

// AutoCountCap is the count cap this host's own calibration derives; 0 means
// "no evidence to derive one from".
//
// With route evidence it is the MINIMUM over the pools of the most common
// route (representativeRoute) - never a sum: a GPU session draws on the
// accelerator AND the CPU pool at once, so the tighter pool is the ceiling and
// adding the other pool's headroom would count streams that cannot exist.
// Without route evidence a table-priced session charges ONE pool, so the pools
// are independent and the cap is their sum (a host may fill its CPU pool with
// software streams and its GPU pool with hardware ones; the per-pool budget
// stays the gate, this is the belt).
//
// It lives HERE, on the limiter that owns both the cost model and the budgets,
// because the admin API once derived a DIFFERENT number from the tallest bench
// rung, so the panel showed an "effective limit" that was not the one enforced.
func (l *VideoSessionLimiter) AutoCountCap() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.routeModel != nil {
		route, ok := representativeRoute(l.routeModel, LadderClassHW)
		if !ok {
			route, ok = representativeRoute(l.routeModel, LadderClassSoftware)
		}
		if !ok {
			return 0
		}
		// h=0 selects the route's lowest measured rung, its cheapest session.
		class := LadderClassSoftware
		if route.Backend != "" {
			class = LadderClassHW
		}
		charges, ok := l.chargesFor(class, 0, transcoding.SourceClass{}, route, 0)
		if !ok {
			return 0
		}
		return l.streamsFitting(charges)
	}
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
