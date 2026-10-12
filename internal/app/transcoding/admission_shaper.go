package transcoding

import (
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/metrics"
)

// StreamPlan is the IMMUTABLE admission decision for one stream: the ladder class
// (route) and ceiling height it was granted. It is minted once, when a stream is
// first admitted, and NEVER mutated - a capacity change mints a fresh plan for the
// NEXT stream (a superseding admission replaces the whole plan), it never rewrites
// a live one. So the ladder a player's manifest was built from and the ladder
// every one of its segments is shaped by are identical BY CONSTRUCTION, not by the
// freshness/bound bookkeeping that used to be the only thing guaranteeing it.
type StreamPlan struct {
	Class   LadderClass
	Ceiling int
}

// VideoAdmissionShaper decides, per stream, the TALLEST ladder rung the host
// can still afford - so a viewer asking for 1440p on a box whose budget only
// has room for 1080p is served 1080p instead of being refused outright.
//
// The hard constraint is the cache-hash invariant: the ladder shapes the
// decision fingerprint, whose hash addresses the segment cache and the ffmpeg
// session. Manifest-time and segment-time MUST therefore see the same ceiling
// for the whole life of a stream, or the player's URLs stop matching the
// bytes. The shaper guarantees it by PINNING the granted ceiling per stream
// key on first sight and returning the pin verbatim afterwards.
//
// A pin is re-evaluated only when BOTH hold:
//   - it has gone stale (no manifest/segment touched it for pinFreshFor), and
//   - no session bound to it still holds a limiter slot.
//
// The first condition keeps the ceiling frozen under an actively-fetching
// player. The second protects a PAUSED player whose session still holds its
// slot: re-evaluating there would price the stream's own reserved cost against
// itself and wrongly lower it mid-play. Conversely, a stream that lost the
// spawn race and was shed re-evaluates naturally on retry - its pin is cold
// and holds nothing - so a refused 1440p viewer comes back and gets 1080p
// instead of being poisoned by their own first attempt.
type VideoAdmissionShaper struct {
	limiter *VideoSessionLimiter

	mu   sync.Mutex
	pins map[string]*streamPin

	// pinFreshFor is how long a pin stays authoritative after its last touch.
	// Segments refresh it every few seconds during playback; Retry-After on a
	// shed is 5s, so a retrying viewer always finds their pin cold.
	pinFreshFor time.Duration
	maxPins     int
	now         func() time.Time
}

type streamPin struct {
	// plan is the IMMUTABLE admission decision (class + ceiling). A fresh or held
	// pin always answers with ITS OWN plan regardless of what class a later
	// GrantRoute call asks about (stability - see StreamPlan's doc); only a pin
	// that goes cold AND unheld is superseded, and then a NEW plan is minted
	// against the newly-requested class. Never mutated in place.
	plan StreamPlan
	// ladder is the frozen shaped ABR ladder EMITTED ONCE for this stream (see
	// FreezeLadder/FrozenLadder). nil until the first shaping freezes it. While the
	// pin is fresh-or-held, every manifest- and segment-time call reads THIS ladder
	// verbatim instead of re-deriving it from mutable inputs (complexity class,
	// capability, CMCD/av1 caps) that could drift mid-stream and break the
	// cache-hash invariant. Superseded together with the plan when the pin goes
	// cold and unheld.
	ladder []transcoding.HLSProfile
	// route is the identity (codec/backend/topology) of the route the stream
	// first resolved to under this pin (NoteRoute). The pin does NOT freeze it
	// - a backend demoted mid-stream must not be kept alive by a pin - it only
	// detects that it moved: counted, logged, then adopted, so a drift is
	// reported once per move, not once per segment.
	route    string
	lastSeen time.Time
	// boundHashes are the decision-fingerprint hashes built under this pin
	// (several can share one: audio tracks x subtitle burn-ins x offsets of
	// the same title). If any still holds a limiter slot, the pin must not
	// move. Bounded: Bind compacts DEAD hashes out of a full set before
	// recording a newcomer, so a live 9th variant is never silently dropped.
	boundHashes []string
	// bindOverflow marks the pathological case where maxBoundHashes variants
	// were SIMULTANEOUSLY live and one more arrived: the set cannot name every
	// live session anymore, so the pin fails SAFE as held until Bind compacts a
	// slot free again OR the pin goes cold (see anyBoundHeld - a flag no later
	// Bind ever revisits would otherwise freeze the plan for the process's life).
	bindOverflow bool
}

const (
	defaultPinFreshFor = 30 * time.Second
	defaultMaxPins     = 1024
	maxBoundHashes     = 8
)

func NewVideoAdmissionShaper(limiter *VideoSessionLimiter) *VideoAdmissionShaper {
	return &VideoAdmissionShaper{
		limiter:     limiter,
		pins:        make(map[string]*streamPin),
		pinFreshFor: defaultPinFreshFor,
		maxPins:     defaultMaxPins,
		now:         time.Now,
	}
}

// RouteGrant is the shaper's full answer for one stream: which ladder class
// (and therefore which producer route and resource pool) it may spawn under,
// and up to which rung. OK=false means nothing fits anywhere right now: the
// caller keeps the natural shape and the shed paths answer the viewer.
type RouteGrant struct {
	Class   LadderClass
	Ceiling int
	OK      bool
}

// GrantRoute grants a route and ceiling for the stream identified by key,
// whose NATURAL class is natural and whose natural ladder offers heights.
// When natural is hw and the GPU pool cannot admit ANY rung, the shaper
// spills the stream to the software route instead of letting it be refused:
// fallbackHeights (lazily evaluated, may be nil to disable the spill) must
// return the heights the SOFTWARE-shaped ladder would offer, since the two
// routes shape different ladders. The grant is pinned like a plain ceiling -
// the route shapes the ladder shapes the fingerprint hash, so it must not
// move for the stream's life (same freshness + bound-session rules).
// RouteAdmission is everything GrantRoute prices a stream on: the natural pool
// class and its ladder heights, the route cost key + source fps the route model
// prices on, and - for a hardware stream - the software SPILL shape and its own
// route key, priced on the CPU pool when the accelerator pool is full.
type RouteAdmission struct {
	Natural         LadderClass
	Heights         []int
	Src             transcoding.SourceClass
	Route           transcoding.RouteCostKey
	SourceFPS       float64
	FallbackHeights func() []int
	FallbackRoute   transcoding.RouteCostKey
}

func (s *VideoAdmissionShaper) GrantRoute(key string, req RouteAdmission) RouteGrant {
	if s == nil || s.limiter == nil || len(req.Heights) == 0 {
		return RouteGrant{}
	}
	natural, heights, src := req.Natural, req.Heights, req.Src
	top := heights[0]
	for _, h := range heights[1:] {
		if h > top {
			top = h
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	if pin, ok := s.pins[key]; ok {
		fresh := now.Sub(pin.lastSeen) < s.pinFreshFor
		if fresh || s.anyBoundHeld(pin) {
			pin.lastSeen = now
			return RouteGrant{Class: pin.plan.Class, Ceiling: pin.plan.Ceiling, OK: true}
		}
		// Cold and holding nothing: this pin belongs to a playback attempt
		// that ended (or never started). Fall through and re-evaluate against
		// the budget as it stands now.
		delete(s.pins, key)
	}

	if granted, ok := s.tallestAdmittable(key, natural, heights, src, req.Route, req.SourceFPS); ok {
		s.evictIfFull(now)
		plan := StreamPlan{Class: natural, Ceiling: granted}
		s.pins[key] = &streamPin{plan: plan, lastSeen: now}
		if granted < top {
			slog.Info("video admission: ladder ceiling forced by load",
				slog.String("stream", key), slog.Int("requested_top", top), slog.Int("granted", granted))
		}
		return RouteGrant{Class: plan.Class, Ceiling: plan.Ceiling, OK: true}
	}

	// The natural pool is full at every rung. A hardware stream may spill to
	// the software route - a different pool AND a different ladder shape, so
	// the fallback heights come from the caller (the software producer's
	// shaping), never guessed from the hw ladder.
	if natural == LadderClassHW && req.FallbackHeights != nil {
		if fh := req.FallbackHeights(); len(fh) > 0 {
			if granted, ok := s.tallestAdmittable(key, LadderClassSoftware, fh, src, req.FallbackRoute, req.SourceFPS); ok {
				s.evictIfFull(now)
				plan := StreamPlan{Class: LadderClassSoftware, Ceiling: granted}
				s.pins[key] = &streamPin{plan: plan, lastSeen: now}
				slog.Info("video admission: gpu pool full, stream spilled to the software route",
					slog.String("stream", key), slog.Int("granted", granted))
				return RouteGrant{Class: plan.Class, Ceiling: plan.Ceiling, OK: true}
			}
		}
	}

	// Nothing fits anywhere. Deliberately NOT pinned: the next attempt must
	// see the budget of ITS moment, not this one's.
	return RouteGrant{}
}

// PinnedClass reports the class key's live pin was granted under, refreshing
// it. Callers that assemble a fingerprint AFTER the grant (hw fields must
// match the granted route, not the natural one) read it here instead of
// re-running the whole grant.
func (s *VideoAdmissionShaper) PinnedClass(key string) (LadderClass, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pin, ok := s.pins[key]
	if !ok {
		return "", false
	}
	pin.lastSeen = s.now()
	return pin.plan.Class, true
}

// FreezeLadder records the shaped ABR ladder emitted for key's stream onto its
// live pin, so subsequent manifest-/segment-time shaping reads it verbatim rather
// than re-deriving it. A no-op when no pin exists for key (a copy request, or a
// refused stream took no slot) or the ladder is empty - nothing to freeze. Stores
// a COPY so a caller mutating its slice afterwards can't corrupt the frozen plan.
// Idempotent-friendly: re-freezing an identical ladder is harmless; the pin's
// plan (class+ceiling) is immutable, so the ladder shaped under it is stable too.
func (s *VideoAdmissionShaper) FreezeLadder(key string, ladder []transcoding.HLSProfile) {
	if s == nil || len(ladder) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pin, ok := s.pins[key]
	if !ok {
		return
	}
	frozen := make([]transcoding.HLSProfile, len(ladder))
	copy(frozen, ladder)
	pin.ladder = frozen
}

// FrozenLadder returns the ladder frozen for key's stream, refreshing the pin's
// freshness like PinnedClass. It answers ONLY while the pin is authoritative - fresh,
// or cold but still held by a live session - matching GrantRoute's stability
// contract exactly: an abandoned (cold + unheld) stream returns nothing so its
// retry re-derives against the budget of its moment. A COPY is returned so the
// caller can shape it further (the CMCD/av1 caps already folded in) without
// touching the stored plan. ok=false when no pin, no frozen ladder, or cold+unheld.
func (s *VideoAdmissionShaper) FrozenLadder(key string) ([]transcoding.HLSProfile, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pin, ok := s.pins[key]
	if !ok || len(pin.ladder) == 0 {
		return nil, false
	}
	now := s.now()
	if now.Sub(pin.lastSeen) >= s.pinFreshFor && !s.anyBoundHeld(pin) {
		return nil, false // cold and unheld: let the caller re-derive (GrantRoute supersedes)
	}
	pin.lastSeen = now
	out := make([]transcoding.HLSProfile, len(pin.ladder))
	copy(out, pin.ladder)
	return out, true
}

// NoteRoute records the route identity a Resolve settled on for key's stream.
// While the pin is authoritative (fresh or held) a different identity than
// the one recorded is a ROUTE DRIFT under a live player - the capability, a
// demotion or a policy toggle moved between its master and its segments; the
// sealed fingerprint in its URLs answers 410 and the player refetches. The
// drift is counted and logged once, and the new identity adopted. A pin that
// went cold and unheld simply records the new route: that is a new stream.
func (s *VideoAdmissionShaper) NoteRoute(key, identity string) {
	if s == nil || identity == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pin, ok := s.pins[key]
	if !ok {
		return
	}
	authoritative := s.now().Sub(pin.lastSeen) < s.pinFreshFor || s.anyBoundHeld(pin)
	if pin.route != "" && pin.route != identity && authoritative {
		metrics.HLSVODRouteDrift.Inc()
		slog.Warn("video admission: route moved under a live stream (the pin keeps its class and ceiling, the player refetches its master)",
			slog.String("stream", key), slog.String("was", pin.route), slog.String("now", identity))
	}
	pin.route = identity
}

// Bind records the decision-fingerprint hash a caller built under key's pin,
// so the pin can tell an active stream from an abandoned attempt.
func (s *VideoAdmissionShaper) Bind(key, fingerprintHash string) {
	if s == nil || fingerprintHash == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pin, ok := s.pins[key]
	if !ok {
		return
	}
	if slices.Contains(pin.boundHashes, fingerprintHash) {
		return
	}
	if len(pin.boundHashes) >= maxBoundHashes {
		// Compact the dead out before dropping anything: variants come and go
		// (a viewer cycling audio tracks), and a stale entry must never cost a
		// LIVE one its protection.
		kept := pin.boundHashes[:0]
		for _, h := range pin.boundHashes {
			if s.limiter != nil && s.limiter.Holds(h) {
				kept = append(kept, h)
			}
		}
		pin.boundHashes = kept
		pin.bindOverflow = len(pin.boundHashes) >= maxBoundHashes
	}
	if len(pin.boundHashes) < maxBoundHashes {
		pin.boundHashes = append(pin.boundHashes, fingerprintHash)
	}
}

// tallestAdmittable walks the offered heights from tallest to shortest and
// returns the first the limiter would admit as a NEW stream right now, in
// ladder class class. Advisory exactly like the shed probes: the
// authoritative reservation stays TryAcquire at spawn - a lost race is shed
// at the segment, and the retry re-evaluates because a shed stream's pin
// goes cold holding nothing.
func (s *VideoAdmissionShaper) tallestAdmittable(key string, class LadderClass, heights []int, src transcoding.SourceClass, route transcoding.RouteCostKey, sourceFPS float64) (int, bool) {
	sorted := make([]int, 0, len(heights))
	seen := make(map[int]bool, len(heights))
	for _, h := range heights {
		if h > 0 && !seen[h] {
			seen[h] = true
			sorted = append(sorted, h)
		}
	}
	slices.SortFunc(sorted, func(a, b int) int { return b - a }) // tallest first
	for _, h := range sorted {
		if s.limiter.CanAdmitRoute(key, class, h, src, route, sourceFPS) {
			return h, true
		}
	}
	return 0, false
}

// anyBoundHeld reports whether a live session still holds this pin's plan.
// Called with s.mu held - it may clear a stale overflow flag.
func (s *VideoAdmissionShaper) anyBoundHeld(pin *streamPin) bool {
	if slices.ContainsFunc(pin.boundHashes, s.limiter.Holds) {
		return true
	}
	if !pin.bindOverflow {
		return false
	}
	// An overflowed set cannot NAME every live session, so it fails safe as
	// held - but only while the pin is warm. Nothing else ever clears the flag
	// (only another Bind on a full set recomputes it), so a stream whose binds
	// stopped froze its plan for the process's life. A pin untouched for
	// pinFreshFor has no manifest or segment behind it whatever the set failed
	// to name: worst case is one re-derivation instead of a permanent freeze.
	if s.now().Sub(pin.lastSeen) < s.pinFreshFor {
		return true
	}
	pin.bindOverflow = false
	return false
}

// evictIfFull bounds the pin map: expired unheld pins first, then the coldest
// UNHELD one. A pin a live session is bound to is never a victim - moving an
// active stream's ceiling desyncs the player's URLs from the bytes, which is
// the exact corruption the pin exists to prevent. When every pin is held, the
// map grows past its soft cap instead: maxPins bounds churn from abandoned
// playback attempts, and held pins are already bounded by the limiter's
// admission (a session must fit a pool budget to hold anything).
// Called with s.mu held.
func (s *VideoAdmissionShaper) evictIfFull(now time.Time) {
	if len(s.pins) < s.maxPins {
		return
	}
	oldestKey, oldest := "", time.Time{}
	for k, p := range s.pins {
		if s.anyBoundHeld(p) {
			continue // a live stream's pin is untouchable
		}
		if now.Sub(p.lastSeen) >= s.pinFreshFor {
			delete(s.pins, k)
			continue
		}
		if oldestKey == "" || p.lastSeen.Before(oldest) {
			oldestKey, oldest = k, p.lastSeen
		}
	}
	if len(s.pins) >= s.maxPins && oldestKey != "" {
		delete(s.pins, oldestKey)
	}
}
