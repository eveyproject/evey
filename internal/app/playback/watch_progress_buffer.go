package playback

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// watchProgressKey identifies a buffered progress entry. Per the backlog,
// the player tick (~5s) repeatedly upserts the same (profile, media) pair -
// folding writes by this key turns N ticks into one DB upsert per flush.
type watchProgressKey struct {
	profileID   shared.ProfileID
	mediaItemID shared.MediaItemID
}

// WatchProgressBuffer coalesces ReportProgressBySession upserts in memory and
// flushes them periodically to the backing repository. The hot path becomes a
// map write (no DB round-trip); a goroutine drains the map every Interval.
//
// Tradeoff: a server crash drops up to FlushInterval worth of progress for
// active streams. Acceptable per the backlog - self-hosted, low write rate.
type WatchProgressBuffer struct {
	repo          playback.ProgressCommandRepo
	flushInterval time.Duration

	mu      sync.Mutex
	pending map[watchProgressKey]*playback.WatchProgress

	// started guards Stop against a deadlock when called without a prior Start:
	// doneCh is only ever closed by the Start goroutine's defer, so waiting on it
	// without a running loop would block forever. Set once by Start.
	started  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// DefaultWatchProgressFlushInterval matches the player tick cadence so a flush
// races at most one in-flight progress update per active stream.
const DefaultWatchProgressFlushInterval = 5 * time.Second

// NewWatchProgressBuffer constructs a buffer with the given flush cadence.
// If interval <= 0, DefaultWatchProgressFlushInterval is used.
func NewWatchProgressBuffer(repo playback.ProgressCommandRepo, interval time.Duration) *WatchProgressBuffer {
	if interval <= 0 {
		interval = DefaultWatchProgressFlushInterval
	}
	return &WatchProgressBuffer{
		repo:          repo,
		flushInterval: interval,
		pending:       make(map[watchProgressKey]*playback.WatchProgress),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Enqueue records the latest progress for (profileID, mediaItemID), folding
// any earlier pending entry. Returns immediately - no I/O on the hot path.
//
// The fold keeps whichever entry has the freshest LastPlayedAt, not simply
// whichever Enqueue call happened to run last: only one entry per key ever
// survives to be flushed, so a stale tick (a second device's in-flight write,
// or a reordered retry) that lost the race to Enqueue must not silently
// overwrite an already-buffered, more recent position - the DB's own
// freshness guard never gets a chance to catch it since it only sees the
// survivor. A fresher tick reporting a lower position (a voluntary rewind)
// still replaces the buffered entry, same semantics as the DB guard.
func (b *WatchProgressBuffer) Enqueue(wp *playback.WatchProgress) {
	if wp == nil {
		return
	}
	k := watchProgressKey{profileID: wp.ProfileID, mediaItemID: wp.MediaItemID}
	b.mu.Lock()
	if existing, ok := b.pending[k]; !ok || !wp.LastPlayedAt.Before(existing.LastPlayedAt) {
		b.pending[k] = wp
	}
	b.mu.Unlock()
}

// Pending returns the count of buffered entries (mainly for tests/metrics).
func (b *WatchProgressBuffer) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

// snapshot drains the pending map and returns the entries to persist.
// Holding the mutex only during the swap keeps the hot path latency-free.
func (b *WatchProgressBuffer) snapshot() []*playback.WatchProgress {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return nil
	}
	out := make([]*playback.WatchProgress, 0, len(b.pending))
	for _, wp := range b.pending {
		out = append(out, wp)
	}
	b.pending = make(map[watchProgressKey]*playback.WatchProgress)
	b.mu.Unlock()
	return out
}

// Flush drains and upserts every pending entry in ONE batch statement (the
// per-entry loop cost one round trip per viewer per tick). If the batch
// fails as a whole, it falls back to per-entry writes so one poisoned row
// cannot cost every viewer's progress; failures are logged and dropped (the
// next tick re-enqueues a fresher position). Returns the number persisted.
func (b *WatchProgressBuffer) Flush(ctx context.Context) int {
	ctx, span := tracing.StartSpan(ctx, "app.playback.WatchProgressBuffer.Flush")
	defer span.End()
	entries := b.snapshot()
	if len(entries) == 0 {
		return 0
	}
	if err := b.repo.UpsertMany(ctx, entries); err == nil {
		return len(entries)
	} else {
		slog.WarnContext(ctx, "watch progress batch flush failed, salvaging per entry", slog.Any("err", err))
	}
	persisted := 0
	for _, wp := range entries {
		if err := b.repo.Upsert(ctx, wp); err != nil {
			slog.WarnContext(ctx, "watch progress flush",
				slog.String("profileID", string(wp.ProfileID)),
				slog.String("mediaItemID", string(wp.MediaItemID)),
				slog.Any("err", err),
			)
			continue
		}
		persisted++
	}
	return persisted
}

// Start launches the periodic flush goroutine. Cancelling ctx OR calling
// Stop() drains the buffer one last time before returning.
//
// Idempotent: calling Start twice spawns a single loop because doneCh is
// only closed once. Callers should treat this as run-once.
func (b *WatchProgressBuffer) Start(ctx context.Context) {
	b.started.Store(true)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic in watch progress buffer flush", slog.Any("recover", r))
			}
			// Final flush on shutdown - uses a fresh context so a cancelled
			// parent doesn't abort the last write. Bound it tightly to avoid
			// blocking shutdown indefinitely on a slow DB.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			b.Flush(flushCtx)
			cancel()
			close(b.doneCh)
		}()
		ticker := time.NewTicker(b.flushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-b.stopCh:
				return
			case <-ticker.C:
				b.safeFlush(ctx)
			}
		}
	}()
}

// safeFlush contains a panic to THAT flush.
//
// The goroutine-level recover above is not enough on its own: it catches the
// panic and then falls into the deferred final-flush-and-close, ending the
// loop for the rest of the process lifetime. Nothing else reports on it, so
// the visible result would be watch progress silently never being persisted
// again - for every profile - while b.pending grows without bound, and Stop()
// still returning cleanly because doneCh is already closed. One bad flush must
// cost one flush.
func (b *WatchProgressBuffer) safeFlush(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "watch progress flush panicked, continuing with the next tick",
				slog.Any("panic", r))
		}
	}()
	b.Flush(ctx)
}

// Stop signals the flush loop to exit and waits for the final drain.
// Safe to call multiple times. Idempotent so graceful shutdown can be
// invoked from several places without coordination.
func (b *WatchProgressBuffer) Stop() {
	// Never started → no flush goroutine will ever close doneCh, so don't wait on
	// it (that was the deadlock). Nothing to drain.
	if !b.started.Load() {
		return
	}
	b.stopOnce.Do(func() {
		close(b.stopCh)
	})
	<-b.doneCh
}
