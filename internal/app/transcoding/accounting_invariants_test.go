package transcoding

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// This is the accounting net for the whole video pipeline.
//
// Six review passes over this subsystem turned up the same shape of bug every
// time: the logic was right, an AUXILIARY LEDGER was wrong. The live-sessions
// gauge incremented after its own reaper could decrement. A killed session
// stayed matchable as a live one. A pin's overflow flag stuck on forever, so a
// stream stayed "held" with nothing holding it. A registry holder was inflated
// by a speculative plan resolution nobody served. None of them sat on the
// critical path, all of them drifted the same way - the system looked busier
// and more pinned than it was - and each was found by reading, one at a time.
//
// They share one testable property: after a burst of realistic churn, every
// ledger must agree with the objects it claims to describe. That is what this
// asserts, and it is why it lives in one test rather than four: any future
// counter that drifts fails HERE, without anyone having to suspect it first.
//
// Deliberately no sleeps: reaps are asynchronous, so the invariants are polled
// to convergence and then reported in full on failure.
func TestAccountingInvariants_HoldAfterChurn(t *testing.T) {
	h := newInvariantHarness(t)

	gaugeBefore := testutil.ToFloat64(metrics.HLSVODSessions)

	h.churn(t)

	// Tear everything down: the end state every ledger must agree on is the
	// simplest one - nothing alive, therefore nothing counted anywhere.
	h.orch.Close()
	h.sw.Close()

	var last string
	require.Eventuallyf(t, func() bool {
		last = h.violations(gaugeBefore)
		return last == ""
	}, 5*time.Second, 10*time.Millisecond, "accounting ledgers never converged:\n%s", &lazyString{&last})

	require.Empty(t, h.violations(gaugeBefore))
}

// violations returns every broken invariant, or "" when the ledgers agree.
// Reported as a list rather than one assertion so a failure names ALL the
// ledgers that drifted, not just the first.
func (h *invariantHarness) violations(gaugeBefore float64) string {
	var out string
	add := func(format string, args ...any) { out += "  - " + fmt.Sprintf(format, args...) + "\n" }

	// 1. The live-sessions gauge describes the sessions that actually exist.
	// Every Inc must be paired with exactly one Dec, whichever exit path the
	// session takes (natural ffmpeg EOF, forced kill, LRU eviction, shutdown)
	// and whatever the ORDER in which those happen.
	live := h.orch.ActiveCount() + h.sw.ActiveSessionCount()
	if got := testutil.ToFloat64(metrics.HLSVODSessions) - gaugeBefore; int(got) != live {
		add("gauge: HLSVODSessions moved by %v, but %d sessions are live", got, live)
	}

	// 2. The video-encode limiter holds exactly the hashes that still have a
	// session. A slot kept past its session's death is capacity the host can
	// never reuse; one released twice lets it over-admit.
	liveHashes := map[string]bool{}
	h.orch.mu.Lock()
	for hash, list := range h.orch.sessions {
		if len(list) > 0 {
			liveHashes[hash] = true
		}
	}
	h.orch.mu.Unlock()
	h.sw.mu.Lock()
	for hash, list := range h.sw.sessions {
		if len(list) > 0 {
			liveHashes[hash] = true
		}
	}
	h.sw.mu.Unlock()
	if h.limiter.Active() != len(liveHashes) {
		add("limiter: %d slots held, %d hashes still have a session", h.limiter.Active(), len(liveHashes))
	}
	for hash := range liveHashes {
		if !h.limiter.Holds(hash) {
			add("limiter: hash %s has a live session but holds no slot", hash)
		}
	}

	// 3. The stream registry's holder counts describe its own entries. A
	// phantom holder makes a real viewer's switch skip the reclaim it exists
	// for; a missing one lets a live stream's encoder be reclaimed under it.
	h.reg.mu.Lock()
	perHash := map[string]int{}
	for _, e := range h.reg.streams {
		perHash[e.hash]++
	}
	for hash, want := range perHash {
		if got := h.reg.holders[hash]; got != want {
			add("registry: holders[%s] = %d, but %d live streams point at it", hash, got, want)
		}
	}
	for hash, got := range h.reg.holders {
		if perHash[hash] == 0 {
			add("registry: holders[%s] = %d with no live stream on it", hash, got)
		}
	}
	h.reg.mu.Unlock()

	// 4. No pin claims to be held by a session that no longer exists. A pin
	// that does is frozen forever: its plan can never be superseded and its
	// entry can never be evicted, so a ceiling forced down during a load spike
	// stays forced for the life of the process.
	h.shaper.mu.Lock()
	now := h.shaper.now()
	for key, pin := range h.shaper.pins {
		if now.Sub(pin.lastSeen) < h.shaper.pinFreshFor {
			continue // fresh: authoritative on freshness alone, nothing to prove
		}
		if h.shaper.anyBoundHeld(pin) {
			add("shaper: pin %s is cold and still reports held, with no slot behind it", key)
		}
	}
	h.shaper.mu.Unlock()

	return out
}

// churn drives the pipeline through every exit path a session has, on both
// producers at once: natural ffmpeg EOF, forced per-hash reclaim, media-wide
// removal, LRU eviction at the per-hash cap, a refusal at the capacity gate,
// and a process that dies the instant it starts (the ordering hazard).
func (h *invariantHarness) churn(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// One stream running maxBoundHashes+1 SIMULTANEOUSLY live variants: the pin
	// can no longer name every session bound to it and starts failing safe as
	// held. Run first, while the limiter is empty, so all of them really hold.
	// Every one then dies - and the pin must not stay "held" forever on the
	// strength of a flag no later Bind will ever revisit.
	overflowKey := "media-overflow|h264|transcode"
	require.True(t, h.shaper.GrantRoute(overflowKey, RouteAdmission{Natural: LadderClassSoftware, Heights: []int{720}}).OK)
	var overflowed []string
	for i := range maxBoundHashes + 1 {
		hash := fmt.Sprintf("overflow-hash-%d", i)
		if h.limiter.TryAcquire(hash, LadderClassSoftware, 720) {
			overflowed = append(overflowed, hash)
		}
		h.shaper.Bind(overflowKey, hash)
	}
	h.shaper.mu.Lock()
	require.True(t, h.shaper.pins[overflowKey].bindOverflow, "the churn must actually overflow the pin")
	h.shaper.mu.Unlock()
	for _, hash := range overflowed {
		h.limiter.Release(hash)
	}

	for i := range 6 {
		fp := h.fingerprint(fmt.Sprintf("media-%d", i), i%3)

		// The shaper grants, the stream binds, the producer spawns - the real
		// order of a playback, so the three ledgers are wired to each other and
		// not just exercised side by side.
		key := fmt.Sprintf("media-%d|h264|transcode", i)
		if g := h.shaper.GrantRoute(key, RouteAdmission{Natural: LadderClassSoftware, Heights: []int{1080, 720}}); g.OK {
			h.shaper.Bind(key, fp.Hash())
		}
		h.reg.Bind(fmt.Sprintf("tok-%d", i), fp.MediaID, fp.Hash())

		if i%2 == 0 {
			require.NoError(t, h.sw.EnsureSegmentRange(ctx, fp, 0, 0))
			// A forward seek past the reach window: a second live position on
			// the same hash, which is what makes the LRU cap fire below.
			require.NoError(t, h.sw.EnsureSegmentRange(ctx, fp, 0, 400))
			require.NoError(t, h.sw.EnsureSegmentRange(ctx, fp, 0, 900))
		} else {
			require.NoError(t, h.orch.EnsureSegmentRange(ctx, fp, 0, 0))
			require.NoError(t, h.orch.EnsureSegmentRange(ctx, fp, 0, 500))
		}
	}

	// A stream switches audio track: the registry names the hash it left, and
	// the reclaim kills that hash's encoders. Aged past reclaimMinIdle, since a
	// hash somebody is still fetching is deliberately spared.
	switched := h.fingerprint("media-0", 0)
	ageSWLastAccess(h.sw, switched.Hash(), 2*reclaimMinIdle)
	newFP := h.fingerprint("media-0", 7)
	if old, ok := h.reg.Bind("tok-0", "media-0", newFP.Hash()); ok {
		h.sw.RemoveHash(old, "stream_superseded")
		h.orch.RemoveHash(old, "stream_superseded")
	}
	require.NoError(t, h.sw.EnsureSegmentRange(ctx, newFP, 0, 0))

	// A playback stops outright.
	h.orch.RemoveSession("media-1")
	h.reg.Forget("tok-1")

	// A session whose ffmpeg dies the instant it is started: the reaper can run
	// its whole teardown before the spawn call has finished its own bookkeeping.
	h.proc.setFailOnExit(domaintranscoding.FailureHWInit)
	dying := h.fingerprint("media-doomed", 0)
	require.NoError(t, h.orch.EnsureSegmentRange(ctx, dying, 0, 0))
	require.NoError(t, h.sw.EnsureSegmentRange(ctx, dying, 0, 0))
	h.proc.setFailOnExit("")

	// The capacity gate refuses: a refused spawn must leave no trace in any
	// ledger. The limiter is sized so this is reached by construction.
	for i := range 12 {
		fp := h.fingerprint(fmt.Sprintf("late-%d", i), 0)
		_ = h.orch.EnsureSegmentRange(ctx, fp, 0, 0)
		_ = h.sw.EnsureSegmentRange(ctx, fp, 0, 0)
	}

	// Every stream ends. The pins go cold with it, which is the state a stuck
	// overflow flag would survive.
	for i := range 6 {
		h.reg.Forget(fmt.Sprintf("tok-%d", i))
	}
	*h.clock = h.clock.Add(2 * defaultPinFreshFor)
}

type invariantHarness struct {
	orch    *SegmentOrchestrator
	sw      *SoftwareProducer
	limiter *VideoSessionLimiter
	shaper  *VideoAdmissionShaper
	reg     *StreamHashRegistry
	proc    *fakeProcess
	clock   *time.Time
}

func newInvariantHarness(t *testing.T) *invariantHarness {
	t.Helper()
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(8)

	orchOpts := testOpts()
	orchOpts.MaxRafalesPerHash = 2
	orch := NewSegmentOrchestrator(cache, proc, stubSource{path: "/m.mkv"}, orchOpts, discardLogger())
	orch.WithVideoSessionLimiter(limiter)

	swOpts := swTestOpts()
	swOpts.MaxSessionsPerHash = 2
	swOpts.NewbornEvictionShield = time.Nanosecond // let the cap actually evict
	sw := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swOpts, discardLogger())
	sw.WithVideoSessionLimiter(limiter)

	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	h := &invariantHarness{
		orch: orch, sw: sw, limiter: limiter, shaper: shaper,
		reg:   NewStreamHashRegistry(time.Minute, 64),
		proc:  proc,
		clock: &clock,
	}
	t.Cleanup(func() { orch.Close(); sw.Close() })
	return h
}

// fingerprint builds a distinct decision hash per (media, audio track) - the
// real shape of a track switch, which re-fingerprints the same viewing session.
func (h *invariantHarness) fingerprint(mediaID string, audioTrack int) DecisionFingerprint {
	fp := DecisionFingerprint{
		MediaID:             mediaID,
		SourcePath:          "/m.mkv",
		VideoCodec:          "h264",
		AudioCodec:          "aac",
		AudioTrack:          audioTrack,
		SubtitleBurnInTrack: -1,
		MaxAudioChannels:    2,
		UseFMP4:             true,
		Profiles: []ProfileFingerprint{
			{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2500, AudioBitrate: 128},
		},
	}
	return fp
}

func (p *fakeProcess) setFailOnExit(c domaintranscoding.FailureClass) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failOnExit = c
}

// lazyString defers reading the violation report until the assertion actually
// formats it, so Eventuallyf shows the LAST poll's state rather than the first.
type lazyString struct{ s *string }

func (l *lazyString) String() string { return *l.s }
