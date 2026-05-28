package transcoding

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// EvictionPolicy configures the LRU sweeper that bounds the on-disk cache.
type EvictionPolicy struct {
	// MaxBytes is the soft cap on total segment cache size on disk. When the
	// sweeper observes the cache above this threshold it deletes the
	// oldest-atime hashes until it falls below MaxBytes - MarginBytes. 0
	// disables eviction (Phase 1 default).
	MaxBytes int64

	// MarginBytes is the headroom below MaxBytes after an eviction pass — gives
	// the next encoder rafale room to land segments before the next sweep
	// hits MaxBytes again. Default = MaxBytes / 10.
	MarginBytes int64

	// SweepInterval is how often the sweeper wakes up to enforce the cap.
	// Default 30 s. Lower interval = tighter bound + more disk reads.
	SweepInterval time.Duration

	// ProtectRecent protects the N most-recent-atime decision hashes from
	// eviction during a sweep — guarantees an actively-watched session
	// (with hits in the last sweep window) is never evicted out from under
	// the player. Default 4.
	ProtectRecent int
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
	return out
}

// StartEviction launches the LRU sweeper in the background. Call once on
// startup, after NewSegmentCache. The returned stop function cancels the
// sweeper goroutine; safe to defer.
//
// The sweeper deletes whole hash directories at a time rather than picking
// individual segments — partial directories would force the orchestrator to
// re-spawn ffmpeg for missing inner segments, defeating the cache. Whole-hash
// eviction also lets us bump atime on access cheaply (atime of the directory
// rather than every file).
func (c *SegmentCache) StartEviction(policy EvictionPolicy, logger *slog.Logger) (stop func()) {
	policy = policy.withDefaults()
	if policy.MaxBytes <= 0 {
		// Disabled — return a no-op stop.
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sweepOnce(p, logger)
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
	if len(hashes) == 0 {
		return
	}
	var total int64
	for _, h := range hashes {
		total += h.Bytes
	}
	if total <= p.MaxBytes {
		return
	}

	// Sort by atime ascending (oldest first). The newest p.ProtectRecent are
	// shielded — they're likely the actively-streaming sessions.
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].LastAccess.Before(hashes[j].LastAccess)
	})
	protected := p.ProtectRecent
	if protected > len(hashes) {
		protected = len(hashes)
	}
	candidates := hashes[:len(hashes)-protected]

	target := p.MaxBytes - p.MarginBytes
	if target < 0 {
		target = 0
	}

	var evicted int
	var freed int64
	for _, h := range candidates {
		if total <= target {
			break
		}
		if err := c.CleanupHash(h.Hash); err != nil {
			logger.Warn("sweepOnce: cleanup failed",
				slog.String("hash", h.Hash),
				slog.Any("err", err))
			continue
		}
		total -= h.Bytes
		freed += h.Bytes
		evicted++
	}

	if evicted > 0 {
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
			return nil
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

// hashSweepGuard is reserved for Phase 3, when we'll let the orchestrator
// register "in-flight" hashes so the sweeper cannot evict a session whose
// ffmpeg is mid-rafale even if it falls outside ProtectRecent.
var _ sync.Mutex
