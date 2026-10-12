package transcoding

// KeyframeOnDemand fills a media's keyframe index AT PLAYBACK NEGOTIATION when
// the paced background backfill hasn't reached the file yet. Without it, a
// freshly added film negotiates TRANSCODE (no keyframes → copy rung
// ineligible), burns CPU re-encoding for the whole viewing, and only the NEXT
// playback benefits from the index - the prod incident of 2026-07-07 ("green
// frames") additionally showed the backfill landing MID-viewing, which the
// vd= decision pin made harmless but still leaves the first viewing on the
// expensive path.
//
// The probe is the mkvgo-fronted KeyframeDispatcher: reading the container's
// own seek index (Matroska Cues / MP4 stss) head-only takes milliseconds, so
// doing it inline at negotiation is imperceptible. Files WITHOUT a usable
// index would fall through to the multi-second ffprobe packet scan - the
// budget context kills that instead of blocking the player start, a negative
// cache remembers the failure so replays don't re-pay the attempt, and the
// paced background worker remains those files' path to eligibility.

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// keyframeOnDemandBudget is how long the NEGOTIATION waits for the probe:
// plenty for a head-only seek-index read (ms), never a player-visible stall.
// The probe itself keeps running past it (see TryIndex) - mkvgo's cue-less
// cluster walk does not honour context cancellation mid-read, so the wait
// and the work must be decoupled or a walk measured at ~14s blocks
// GetStreamInfo for its whole duration.
const keyframeOnDemandBudget = 3 * time.Second

// keyframeOnDemandHardCap bounds the DETACHED probe+persist so an abandoned
// background attempt cannot outlive its usefulness (and kills the exec'd
// ffprobe fallback if the dispatcher dropped to it).
const keyframeOnDemandHardCap = 3 * time.Minute

// keyframeOnDemandRetryAfter is the negative-cache TTL: a file that yielded
// no index is not re-attempted inline before this delay (the background
// worker owns the slow path).
const keyframeOnDemandRetryAfter = time.Hour

// keyframeOnDemandMaxConcurrent bounds how many detached probes run at once
// ACROSS all media. The inflight set only dedups the SAME id; without a global
// cap a burst of first-plays on freshly added files (each a multi-second mkvgo
// cluster walk on a cue-less MKV) would spawn one detached walk per file and
// saturate the I/O of the weak NAS the project targets. When the slots are
// full the negotiation returns false immediately (this playback transcodes,
// pinned as usual) and the paced background worker indexes the file later.
const keyframeOnDemandMaxConcurrent = 2

type KeyframeOnDemand struct {
	store  KeyframeBackfillStore
	prober KeyframeProber
	logger *slog.Logger
	budget time.Duration
	sem    chan struct{}

	// Optional CFR/VFR classification riding the same walk (mirrors the
	// backfill worker's WithFrameRateAnalyzer): files indexed here never
	// reappear in the backfill queue, so without this the frame-rate mode
	// would stay unknown exactly for on-demand- and ingest-warmed files.
	frAnalyzer FrameRateAnalyzer
	frStore    FrameRateStore

	// activePlayback, when set, reports live playback sessions. While any is
	// running, TryIndex skips the detached probe entirely: a cue-less MKV's
	// keyframe walk is a multi-second CPU/I/O burst that mkvgo can't cancel
	// mid-read, so on a weak NAS it would starve the very playback that
	// triggered it. The paced background backfill worker (which also yields to
	// playback) indexes the file once nobody is watching.
	activePlayback func() int

	mu       sync.Mutex
	inflight map[string]struct{}  // media IDs probed right now (skip duplicates)
	failed   map[string]time.Time // media ID → last failed attempt
}

func NewKeyframeOnDemand(store KeyframeBackfillStore, prober KeyframeProber, logger *slog.Logger) *KeyframeOnDemand {
	if logger == nil {
		logger = slog.Default()
	}
	return &KeyframeOnDemand{
		store:    store,
		prober:   prober,
		logger:   logger.With("component", "keyframe_ondemand"),
		budget:   keyframeOnDemandBudget,
		sem:      make(chan struct{}, keyframeOnDemandMaxConcurrent),
		inflight: make(map[string]struct{}),
		failed:   make(map[string]time.Time),
	}
}

// WithFrameRateAnalyzer arms the optional CFR/VFR classification performed
// after a successful index (nil = skip, previous behavior). Best-effort:
// analysis failures never fail the index itself.
func (k *KeyframeOnDemand) WithFrameRateAnalyzer(a FrameRateAnalyzer, store FrameRateStore) *KeyframeOnDemand {
	k.frAnalyzer = a
	k.frStore = store
	return k
}

// WithActivePlayback wires the live playback-session counter: while any session
// is running, TryIndex defers to the paced background worker instead of
// spawning a probe that would compete with the active stream for the CPU.
func (k *KeyframeOnDemand) WithActivePlayback(count func() int) *KeyframeOnDemand {
	k.activePlayback = count
	return k
}

// WithInlineBudget overrides how long TryIndex waits before detaching
// (tests; production keeps the default). Chainable.
func (k *KeyframeOnDemand) WithInlineBudget(d time.Duration) *KeyframeOnDemand {
	if d > 0 {
		k.budget = d
	}
	return k
}

// WithMaxConcurrent overrides the global detached-probe cap (tests; production
// keeps the default). Chainable.
func (k *KeyframeOnDemand) WithMaxConcurrent(n int) *KeyframeOnDemand {
	if n > 0 {
		k.sem = make(chan struct{}, n)
	}
	return k
}

// TryIndex attempts the index for mediaID's file and reports whether
// keyframes were persisted IN TIME for this negotiation (the caller should
// then re-read streams so its decision sees them). The probe runs on a
// DETACHED goroutine: when it exceeds the inline budget the negotiation
// returns false immediately (this playback transcodes, pinned as usual) and
// the index lands in the background for the next one. Always safe to call:
// duplicate attempts no-op via the inflight set, recent failures via the
// negative cache, and the store write is NULL-guarded so racing the
// background worker is harmless.
func (k *KeyframeOnDemand) TryIndex(ctx context.Context, mediaID, filePath string) bool {
	if k == nil || filePath == "" {
		return false
	}
	// Someone is watching: don't add a keyframe walk on top of a live transcode
	// on a weak NAS. Leave it to the paced background worker (it yields to
	// playback too); this playback transcodes, pinned as usual. Debug: every
	// segment of that playback asks again, an INFO line each was log noise.
	if k.activePlayback != nil && k.activePlayback() > 0 {
		k.logger.Debug("keyframe on-demand: playback active, leaving to the background worker",
			slog.String("media_id", mediaID))
		return false
	}
	k.mu.Lock()
	if _, busy := k.inflight[mediaID]; busy {
		k.mu.Unlock()
		return false
	}
	if t, ok := k.failed[mediaID]; ok {
		if time.Since(t) < keyframeOnDemandRetryAfter {
			k.mu.Unlock()
			return false
		}
		// Expired: purge eagerly. Without this the negative cache only ever
		// grew - one stale entry per failed media, kept for the process
		// lifetime even after the file was repaired or re-probed.
		delete(k.failed, mediaID)
	}
	k.inflight[mediaID] = struct{}{}
	k.mu.Unlock()

	// Claim a global probe slot without blocking: when every slot is taken the
	// negotiation must not queue (that would stall the player) nor spawn an
	// unbounded walk - it releases its inflight claim and lets the background
	// worker own this file.
	select {
	case k.sem <- struct{}{}:
	default:
		k.mu.Lock()
		delete(k.inflight, mediaID)
		k.mu.Unlock()
		k.logger.Info("keyframe on-demand: probe slots full, leaving to the background worker",
			slog.String("media_id", mediaID))
		return false
	}

	done := make(chan bool, 1)
	go func() {
		defer func() {
			// A panic in the prober/store must not crash the whole server (this
			// runs during live playback negotiation). Recover, unblock the waiter.
			if r := recover(); r != nil {
				k.logger.Error("keyframe on-demand: panic in detached probe",
					slog.Any("panic", r), slog.String("media_id", mediaID))
				select {
				case done <- false:
				default:
				}
			}
			<-k.sem
			k.mu.Lock()
			delete(k.inflight, mediaID)
			k.mu.Unlock()
		}()
		// Detached from the request context on purpose: a probe that outlives
		// the negotiation must still finish and persist for the NEXT playback.
		bg, cancel := context.WithTimeout(context.Background(), keyframeOnDemandHardCap)
		defer cancel()
		done <- k.index(bg, mediaID, filePath)
	}()

	select {
	case ok := <-done:
		return ok
	case <-time.After(k.budget):
		k.logger.Info("keyframe on-demand: probe exceeds the inline budget, finishing in background for the next playback",
			slog.String("media_id", mediaID))
		return false
	case <-ctx.Done():
		return false
	}
}

// index probes and persists; returns true when the index is available. Sets
// the negative cache on failure. Runs on the detached goroutine.
func (k *KeyframeOnDemand) index(ctx context.Context, mediaID, filePath string) bool {
	start := time.Now()
	kf, err := k.prober.ProbeKeyframes(ctx, filePath)
	if err != nil || len(kf) == 0 {
		k.mu.Lock()
		k.failed[mediaID] = time.Now()
		k.mu.Unlock()
		// Expected for files without a readable seek index (the exec'd ffprobe
		// fallback dies with the hard cap): the paced background worker owns
		// those. Debug, not warn - fires once per file per TTL.
		k.logger.Debug("keyframe on-demand: probe failed, leaving to the background worker",
			slog.String("media_id", mediaID), slog.Duration("took", time.Since(start)), slog.Any("err", err))
		return false
	}
	n, err := k.store.SetKeyframes(ctx, shared.MediaItemID(mediaID), kf)
	if err != nil {
		k.mu.Lock()
		k.failed[mediaID] = time.Now()
		k.mu.Unlock()
		k.logger.Warn("keyframe on-demand: persist failed",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false
	}
	// Success on either branch below: drop any stale negative-cache entry so
	// a previously failed media doesn't keep its tombstone after being fixed.
	k.mu.Lock()
	delete(k.failed, mediaID)
	k.mu.Unlock()
	if n == 0 {
		// The background worker beat us to it - the index exists either way.
		return true
	}
	k.classifyFrameRate(ctx, mediaID, filePath)
	k.logger.Info("keyframe on-demand: indexed at negotiation",
		slog.String("media_id", mediaID),
		slog.Int("keyframes", len(kf)),
		slog.Duration("took", time.Since(start)))
	return true
}

// classifyFrameRate persists the CFR/VFR mode alongside a fresh index when
// the analyzer is armed. Errors and unknown modes are debug-logged and
// dropped - the copy gate fails open on "" by design.
func (k *KeyframeOnDemand) classifyFrameRate(ctx context.Context, mediaID, filePath string) {
	if k.frAnalyzer == nil || k.frStore == nil {
		return
	}
	mode, err := k.frAnalyzer.AnalyzeFrameRateMode(ctx, filePath)
	if err != nil || mode == "" {
		k.logger.Debug("keyframe on-demand: frame-rate classification skipped",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return
	}
	if _, err := k.frStore.SetFrameRateMode(ctx, shared.MediaItemID(mediaID), mode, FrameRateAlgoVersion); err != nil {
		k.logger.Warn("keyframe on-demand: frame-rate persist failed",
			slog.String("media_id", mediaID), slog.Any("err", err))
	}
}
