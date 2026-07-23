package playback

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Cast progress derivation (user-requested 2026-07-20): once the sender's
// browser closes, a cast receiver keeps fetching media on its own with its
// cast token and NOTHING reports progress anymore - "continue watching" then
// ignores everything watched on the TV. The receiver's fetches are the only
// signal left, so the serving handlers feed them here and this tracker turns
// them into watch-progress rows.
const (
	// castBufferMarginMs compensates the receiver's read-ahead: a Chromecast
	// buffers well ahead of the playhead, so the fetch high-water overstates
	// the position. Subtracting a fixed margin means resuming slightly BEFORE
	// what was watched - the failure mode users forgive (a few rewatched
	// seconds) instead of the one they don't (skipped content).
	castBufferMarginMs = int64(30_000)
	// castFlushInterval paces the derived reports; the write path already
	// folds them through the WatchProgressBuffer, this just bounds how stale
	// a derived position can be.
	castFlushInterval = 15 * time.Second
	// castEntryIdleTTL drops a cast whose receiver went quiet (stopped, film
	// over, network gone) after one final flush. Comfortably above segment
	// cadence: a PLAYING receiver fetches every few seconds.
	castEntryIdleTTL = 2 * time.Minute
)

// CastProgressReporter persists one derived position - in production,
// CommandService.ReportProgress with completed=false (the 90% auto-complete
// classification still applies downstream).
type CastProgressReporter func(ctx context.Context, profileID, mediaItemID string, positionMs int64) error

type castEntry struct {
	profileID   string
	mediaItemID string
	highMs      int64 // furthest media time the receiver has fetched
	reportedMs  int64 // last position successfully reported
	lastSeen    time.Time
}

// CastProgressTracker aggregates receiver fetches per (profile, media) and
// periodically reports position = high-water - margin, monotonically. Safe
// for concurrent use by the serving handlers.
type CastProgressTracker struct {
	mu      sync.Mutex
	entries map[string]*castEntry
	report  CastProgressReporter
	now     func() time.Time
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func NewCastProgressTracker(report CastProgressReporter) *CastProgressTracker {
	return &CastProgressTracker{
		entries: make(map[string]*castEntry),
		report:  report,
		now:     time.Now,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// ObserveFetch records that the receiver identified by (profileID,
// mediaItemID) has fetched media up to endMs. Cheap: a map upsert under a
// mutex, called once per served segment.
func (t *CastProgressTracker) ObserveFetch(profileID, mediaItemID string, endMs int64) {
	if t == nil || profileID == "" || mediaItemID == "" || endMs <= 0 {
		return
	}
	key := profileID + "|" + mediaItemID
	t.mu.Lock()
	e := t.entries[key]
	if e == nil {
		e = &castEntry{profileID: profileID, mediaItemID: mediaItemID}
		t.entries[key] = e
	}
	if endMs > e.highMs {
		e.highMs = endMs
	}
	e.lastSeen = t.now()
	t.mu.Unlock()
}

// Start runs the flush loop until ctx ends or Stop is called.
func (t *CastProgressTracker) Start(ctx context.Context) {
	go func() {
		defer close(t.done)
		ticker := time.NewTicker(castFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.stop:
				return
			case <-ticker.C:
				t.flushOnce(ctx)
			}
		}
	}()
}

// Stop ends the loop and performs one final flush: the common end of a cast
// is the user turning everything off, and that last position is the one that
// matters. Safe to call once; blocks until the loop exited.
func (t *CastProgressTracker) Stop() {
	t.once.Do(func() {
		close(t.stop)
		<-t.done
		// Detached final flush: the service ctx is already ending; losing the
		// last positions to its cancellation would defeat the whole tracker.
		t.flushOnce(context.WithoutCancel(context.Background()))
	})
}

// flushOnce reports every entry whose derived position advanced, and drops
// entries whose receiver has gone quiet (after their own final report).
func (t *CastProgressTracker) flushOnce(ctx context.Context) {
	type pending struct {
		e     *castEntry
		posMs int64
		drop  bool
	}
	now := t.now()
	t.mu.Lock()
	var work []pending
	for key, e := range t.entries {
		idle := now.Sub(e.lastSeen) > castEntryIdleTTL
		pos := e.highMs - castBufferMarginMs
		if pos > e.reportedMs && pos > 0 {
			work = append(work, pending{e: e, posMs: pos, drop: idle})
		}
		if idle {
			// Dead cast: drop it now; its final report (if any) is in work.
			// A failed final report is accepted - the receiver is gone, there
			// will be no better signal to retry with.
			delete(t.entries, key)
		}
	}
	t.mu.Unlock()

	for _, w := range work {
		if err := t.report(ctx, w.e.profileID, w.e.mediaItemID, w.posMs); err != nil {
			slog.Warn("cast progress: derived report failed",
				slog.String("profile_id", w.e.profileID),
				slog.String("media_item_id", w.e.mediaItemID),
				slog.Any("err", err))
			continue // entry keeps reportedMs, the next flush retries
		}
		t.mu.Lock()
		if w.posMs > w.e.reportedMs {
			w.e.reportedMs = w.posMs
		}
		t.mu.Unlock()
	}
}
