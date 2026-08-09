package transcoding

import (
	"context"
	"errors"
	"fmt"
	"github.com/evey-project/evey/internal/pkg/bgloop"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/evey-project/evey/internal/pkg/metrics"
)

// EvictionPolicy configures the LRU sweeper that bounds the on-disk cache.
type EvictionPolicy struct {
	// MaxBytes is the soft cap on total segment cache size on disk. When the
	// sweeper observes the cache above this threshold it deletes the
	// oldest-atime hashes until it falls below MaxBytes - MarginBytes. 0
	// disables eviction (Phase 1 default).
	MaxBytes int64

	// MarginBytes is the headroom below MaxBytes after an eviction pass - gives
	// the next encoder rafale room to land segments before the next sweep
	// hits MaxBytes again. Default = MaxBytes / 10.
	MarginBytes int64

	// SweepInterval is how often the sweeper wakes up to enforce the cap.
	// Default 30 s. Lower interval = tighter bound + more disk reads.
	SweepInterval time.Duration

	// ProtectRecent protects the N most-recent-atime decision hashes from
	// eviction during a sweep - guarantees an actively-watched session
	// (with hits in the last sweep window) is never evicted out from under
	// the player. Default 4.
	ProtectRecent int

	// ActivePlayback reports how many media are being read right now, across
	// every serving path (see ProducerSelector.ActivePlaybackCount). When it is
	// > 0 the sweeper backs off to PlaybackSweepInterval.
	//
	// Backing off rather than pausing outright is deliberate: eviction is the
	// only thing holding the cache under its cap, and a session WRITES segments
	// as it plays, so a sweeper that stopped for a whole film could let the disk
	// fill. Optional; nil keeps the old fixed cadence.
	ActivePlayback func() int

	// PlaybackSweepInterval is the cadence used while playback is active.
	// Default 5 min, against 30 s idle.
	//
	// What this avoids is the sweep's FULL WALK of the cache root - one lstat
	// per file, tens of thousands of them at the 50 GB default cap - which runs
	// on every tick whatever the fill level, because the total is only known
	// once the walk is done. On a weak NAS whose cache shares a spindle with the
	// media, that is the same disk the viewer is reading from.
	PlaybackSweepInterval time.Duration
}

// shouldSweep decides whether this tick pays for a full cache walk.
//
// While playback is active the sweeper drops to PlaybackSweepInterval, because
// the walk lstats every file under the cache root on EVERY tick regardless of
// fill (the total is only known once the walk finishes) - background disk work
// in contention with the viewer's own reads.
//
// The first tick always sweeps (lastSweep zero): a server that boots already
// over its cap must not stay unbounded just because someone is watching.
func (p EvictionPolicy) shouldSweep(now, lastSweep time.Time) bool {
	if p.ActivePlayback == nil || p.ActivePlayback() == 0 {
		return true
	}
	if lastSweep.IsZero() {
		return true
	}
	return now.Sub(lastSweep) >= p.PlaybackSweepInterval
}

func (p EvictionPolicy) withDefaults() EvictionPolicy {
	out := p
	if out.MaxBytes > 0 && out.MarginBytes <= 0 {
		out.MarginBytes = out.MaxBytes / 10
	}
	if out.SweepInterval <= 0 {
		out.SweepInterval = 30 * time.Second
	}
	if out.ProtectRecent <= 0 {
		out.ProtectRecent = 4
	}
	if out.PlaybackSweepInterval <= 0 {
		out.PlaybackSweepInterval = 5 * time.Minute
	}
	return out
}

// StartEviction launches the LRU sweeper in the background. Call once on
// startup, after NewSegmentCache. The returned stop function cancels the
// sweeper goroutine; safe to defer.
//
// The sweeper deletes whole hash directories at a time rather than picking
// individual segments - partial directories would force the orchestrator to
// re-spawn ffmpeg for missing inner segments, defeating the cache. Whole-hash
// eviction also lets us bump atime on access cheaply (atime of the directory
// rather than every file).
func (c *SegmentCache) StartEviction(policy EvictionPolicy, logger *slog.Logger) (stop func()) {
	policy = policy.withDefaults()
	if policy.MaxBytes <= 0 {
		// Disabled - return a no-op stop.
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go c.evictionLoop(ctx, policy, logger, done)
	return func() {
		cancel()
		<-done
	}
}

func (c *SegmentCache) evictionLoop(ctx context.Context, p EvictionPolicy, logger *slog.Logger, done chan struct{}) {
	defer close(done)
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "segment_cache_evict")
	ticker := time.NewTicker(p.SweepInterval)
	defer ticker.Stop()
	var lastSweep time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if !p.shouldSweep(now, lastSweep) {
				continue
			}
			lastSweep = now
			bgloop.Tick("transcoding.segmentCacheEvict", func() { c.sweepOnce(p, logger) })
		}
	}
}

// sweepOnce computes total disk usage under the cache root, and if above
// MaxBytes, deletes whole hash directories in least-recently-used order until
// usage drops below MaxBytes - MarginBytes. The N most-recent hashes are
// protected so an actively-played session is never evicted.
func (c *SegmentCache) sweepOnce(p EvictionPolicy, logger *slog.Logger) {
	hashes, err := c.listHashUsage()
	if err != nil {
		logger.Warn("sweepOnce: list hashes failed", slog.Any("err", err))
		return
	}
	var total int64
	for _, h := range hashes {
		total += h.Bytes
	}
	// Publish the footprint on EVERY sweep, not just the ones that evict:
	// setting it only past the two early returns below left the gauge stuck at 0
	// since boot (or frozen on the last over-cap reading) during normal
	// operation, which is exactly when an operator wants to read it.
	metrics.HLSVODCacheBytes.Set(float64(total))
	if len(hashes) == 0 {
		return
	}
	if total <= p.MaxBytes {
		return
	}

	// Sort by atime ascending (oldest first). The newest p.ProtectRecent are
	// shielded - they're likely the actively-streaming sessions.
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].LastAccess.Before(hashes[j].LastAccess)
	})
	protected := min(p.ProtectRecent, len(hashes))
	candidates := hashes[:len(hashes)-protected]

	target := max(p.MaxBytes-p.MarginBytes, 0)

	var evicted int
	var freed int64
	for _, h := range candidates {
		if total <= target {
			break
		}
		ok, err := c.cleanupHashIfIdle(h.Hash)
		if err != nil {
			logger.Warn("sweepOnce: cleanup failed",
				slog.String("hash", h.Hash),
				slog.Any("err", err))
			continue
		}
		if !ok {
			// A live session is streaming this hash - skip it. The cache may
			// briefly exceed MaxBytes until the session ends; on a small NAS
			// cache that's the right tradeoff vs. stalling a player to 503.
			continue
		}
		total -= h.Bytes
		freed += h.Bytes
		evicted++
	}

	metrics.HLSVODCacheBytes.Set(float64(total))
	if evicted > 0 {
		metrics.HLSVODCacheEvictedBytes.Add(float64(freed))
		logger.Info("sweepOnce: evicted",
			slog.Int("hashes", evicted),
			slog.Int64("freed_bytes", freed),
			slog.Int64("total_after_bytes", total),
			slog.Int("protected", protected))
	}
}

// HashUsage is a single decision-hash bucket's disk footprint + last-access
// signal. The sweeper sorts by LastAccess to drive LRU eviction.
type HashUsage struct {
	Hash       string
	Bytes      int64
	LastAccess time.Time
}

// listHashUsage walks the cache root one level deep, totalling each hash
// directory's segment bytes and recording the freshest mtime across its
// files as LastAccess. LRU on directory atime would be cheaper (one stat per
// dir) but Linux mounted with `noatime` (common for media servers) would
// freeze it; mtime is updated on every segment write by ffmpeg, so it's a
// safer proxy for "this hash is still being produced into".
func (c *SegmentCache) listHashUsage() ([]HashUsage, error) {
	entries, err := os.ReadDir(c.baseDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("listHashUsage: %w", err)
	}
	var out []HashUsage
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.baseDir, e.Name())
		size, mt, err := dirSizeAndMtime(dir)
		if err != nil {
			continue
		}
		out = append(out, HashUsage{Hash: e.Name(), Bytes: size, LastAccess: mt})
	}
	// Merge in the access-time registry: a hash that is being READ (segments
	// served) but no longer WRITTEN keeps a fresh LastAccess. max(mtime, access)
	// is strictly >= the mtime-only value, so eviction is never MORE aggressive
	// than before - an actively-watched copy title (frozen mtime) stays warm.
	// Done after the directory I/O so the lock is held only for the map merge.
	// defer: this sweep runs under bgloop.Tick, which recovers panics. A plain
	// RUnlock would not be reached and the read lock would stay held, blocking
	// every writer of the cache.
	func() {
		c.mu.RLock()
		defer c.mu.RUnlock()
		for i := range out {
			if at, ok := c.accessTimes[out[i].Hash]; ok && at.After(out[i].LastAccess) {
				out[i].LastAccess = at
			}
		}
	}()
	return out, nil
}

func dirSizeAndMtime(dir string) (int64, time.Time, error) {
	var total int64
	var newest time.Time
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // vanished-mid-walk entries are skipped by design
		}
		total += info.Size()
		if mt := info.ModTime(); mt.After(newest) {
			newest = mt
		}
		return nil
	})
	return total, newest, err
}

// TotalBytes returns the current cumulative size of every segment under the
// cache root. Cheap diagnostic (walked once per call); not meant for hot paths.
func (c *SegmentCache) TotalBytes() (int64, error) {
	hashes, err := c.listHashUsage()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, h := range hashes {
		total += h.Bytes
	}
	return total, nil
}

// In-flight protection (formerly the reserved "Phase 3" guard) is now live:
// sweepOnce evicts via cleanupHashIfIdle, which skips any hash with a non-zero
// pendingRefs so a mid-rafale session is never evicted even when it falls
// outside ProtectRecent.
