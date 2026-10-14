package transcoding

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"time"

	"github.com/eveyproject/evey/internal/pkg/bgloop"
	"github.com/eveyproject/evey/internal/pkg/metrics"
)

// CacheBudget is the one sweeper of the transcode cache. It holds the segment
// cache (per-decision-hash directories with live sessions) and the flat file
// caches (fast-start remuxes, compat downloads) under ONE budget, the admin's
// "transcode cache budget", instead of three sweepers each with a cap of its
// own that added up to far more than the operator ever set. On a small box the
// cache lives on the system disk next to everything else the operator runs,
// so the budget is an absolute number of bytes, never a share of the volume,
// and a free-space floor keeps the disk from filling whatever the budget says.
//
// One sweep, three passes, in this order:
//  1. Age: a download file untouched for DownloadTTL goes, whatever the cap.
//     It was produced for one phone and served; keeping it "warm" for a week
//     is generous already. A file a player holds (FileLeases) is spared in
//     every pass.
//  2. Floor: while the filesystem has less than MinFreeBytes free, the least
//     recently used entry goes, whatever the cap. Only a hash a live session
//     serves, or a file a player holds, is spared.
//  3. Cap: while the total sits above MaxBytes, the least recently used entry
//     goes until it falls under MaxBytes - MarginBytes. An entry accessed
//     within ProtectWindow is spared here (a viewer just pulled from it: a
//     remux a player ranges through is reopened on every request), a live
//     session always is.
//
// Least recently used is judged across every kind of entry with the same
// clock: a file's mtime (the remux and download handlers touch it on every
// serve) or a hash's last write or Touch.
type CacheBudget struct {
	segments *SegmentCache
	files    []*RemuxCacheJanitor
	// oversize names the entries already reported as larger than the cap on
	// their own, so the WARN fires once per entry and not on every sweep.
	// Touched by the sweeper goroutine only.
	oversize map[string]bool
}

// NewCacheBudget builds the sweeper over the segment cache and any flat file
// caches sharing its budget.
func NewCacheBudget(segments *SegmentCache, files ...*RemuxCacheJanitor) *CacheBudget {
	return &CacheBudget{segments: segments, files: files}
}

// Start launches the background sweeper. MaxBytes <= 0 disables the cap; the
// age and floor passes still run, since neither depends on it. The returned
// stop joins the goroutine and is safe to defer.
func (b *CacheBudget) Start(policy EvictionPolicy, logger *slog.Logger) (stop func()) {
	policy = policy.withDefaults()
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "cache_budget")
	if policy.FreeBytes == nil && b.segments != nil {
		policy.FreeBytes = b.segments.FreeBytes
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(policy.SweepInterval)
		defer ticker.Stop()
		var lastSweep time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if !policy.shouldSweep(now, lastSweep) {
					continue
				}
				lastSweep = now
				bgloop.Tick("transcoding.cacheBudget", func() { b.sweepOnce(policy, logger, now) })
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// budgetEntry is one evictable thing: a segment hash or a cache file.
type budgetEntry struct {
	// hash names a segment cache directory; path a flat cache file. Exactly
	// one is set.
	hash       string
	path       string
	bytes      int64
	lastAccess time.Time
	// leases holds the files a player is reading (a file entry only).
	leases *FileLeases
}

func (e budgetEntry) isFile() bool { return e.path != "" }

func (b *CacheBudget) list(logger *slog.Logger) (entries []budgetEntry, total int64) {
	if b.segments != nil {
		hashes, err := b.segments.listHashUsage()
		if err != nil {
			logger.Warn("cache budget: list segments failed", slog.Any("err", err))
		}
		for _, h := range hashes {
			entries = append(entries, budgetEntry{hash: h.Hash, bytes: h.Bytes, lastAccess: h.LastAccess})
			total += h.Bytes
		}
	}
	for _, j := range b.files {
		files, err := j.listFiles()
		if err != nil {
			logger.Warn("cache budget: list files failed", slog.String("dir", j.dir), slog.Any("err", err))
			continue
		}
		for _, f := range files {
			entries = append(entries, budgetEntry{path: f.path, bytes: f.bytes, lastAccess: f.mtime, leases: j.leases})
			total += f.bytes
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].lastAccess.Before(entries[j].lastAccess) })
	return entries, total
}

// evict removes one entry. ok is false when it must stay: a hash a live
// session serves, a file a player holds (FileLeases). An unleased file is
// removed even while a download reads it - the open descriptor keeps the
// bytes until the client is done, and the next status request answers
// "absent", so the phone asks for a fresh production.
func (b *CacheBudget) evict(e budgetEntry, now time.Time) (ok bool, err error) {
	if e.isFile() {
		if e.leases.Held(e.path, now) {
			return false, nil
		}
		if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		return true, nil
	}
	return b.segments.cleanupHashIfIdle(e.hash)
}

// sweepOnce runs the three passes. Every sweep publishes the footprint, not
// only the ones that evict, so the gauge reads true during normal operation.
func (b *CacheBudget) sweepOnce(p EvictionPolicy, logger *slog.Logger, now time.Time) {
	entries, total := b.list(logger)
	metrics.HLSVODCacheBytes.Set(float64(total))
	if len(entries) == 0 {
		return
	}
	var freed int64
	var evicted int
	remove := func(e budgetEntry, reason string) bool {
		ok, err := b.evict(e, now)
		if err != nil {
			logger.Warn("cache budget: evict failed", slog.String("reason", reason),
				slog.String("hash", e.hash), slog.String("path", e.path), slog.Any("err", err))
			return false
		}
		if !ok {
			return false
		}
		total -= e.bytes
		freed += e.bytes
		evicted++
		return true
	}
	gone := make(map[int]bool, len(entries))

	// 1. Age.
	if p.DownloadTTL > 0 {
		cutoff := now.Add(-p.DownloadTTL)
		for i, e := range entries {
			if e.isFile() && e.lastAccess.Before(cutoff) && remove(e, "age") {
				gone[i] = true
			}
		}
	}

	// 2. Floor.
	if p.MinFreeBytes > 0 && p.FreeBytes != nil {
		if free, err := p.FreeBytes(); err == nil && free < p.MinFreeBytes {
			want := p.MinFreeBytes + p.MarginBytes
			for i, e := range entries {
				if free >= want {
					break
				}
				if gone[i] {
					continue
				}
				if remove(e, "free_space_floor") {
					gone[i] = true
					free += e.bytes
				}
			}
			if free < want {
				logger.Warn("cache budget: free space still under the floor after the sweep",
					slog.Int64("free_mb", free>>20), slog.Int64("floor_mb", p.MinFreeBytes>>20))
			}
		}
	}

	// 3. Cap.
	if p.MaxBytes > 0 && total > p.MaxBytes {
		target := max(p.MaxBytes-p.MarginBytes, 0)
		protectSince := now.Add(-p.ProtectWindow)
		for i, e := range entries {
			if total <= target {
				break
			}
			if gone[i] {
				continue
			}
			if e.lastAccess.After(protectSince) {
				continue // a viewer pulled from it just now
			}
			if remove(e, "cap") {
				gone[i] = true
			}
		}
	}

	b.warnOversize(entries, gone, p, logger, now)

	metrics.HLSVODCacheBytes.Set(float64(total))
	if evicted > 0 {
		metrics.HLSVODCacheEvictedBytes.Add(float64(freed))
		logger.Info("cache budget: evicted",
			slog.Int("entries", evicted),
			slog.Int64("freed_bytes", freed),
			slog.Int64("total_after_bytes", total),
			slog.Int64("max_bytes", p.MaxBytes))
	}
}

// warnOversize reports, once, an entry larger than the whole cap. The cap pass
// cannot hold it: while a viewer reads it (the protect window, a live encoder)
// it is skipped and keeps growing, bounded by the free-space floor alone; once
// idle it goes whole, the only entry left or not. Nothing loops, but the
// budget an admin set is not the one the disk sees, and nothing said so.
func (b *CacheBudget) warnOversize(entries []budgetEntry, gone map[int]bool, p EvictionPolicy, logger *slog.Logger, now time.Time) {
	if p.MaxBytes <= 0 {
		return
	}
	still := make(map[string]bool, len(b.oversize))
	for i, e := range entries {
		if gone[i] || e.bytes <= p.MaxBytes {
			continue
		}
		key := e.hash + e.path
		still[key] = true
		if b.oversize[key] {
			continue
		}
		logger.Warn("cache budget: one entry is larger than the whole cache budget",
			slog.String("hash", e.hash), slog.String("path", e.path),
			slog.Int64("entry_bytes", e.bytes), slog.Int64("max_bytes", p.MaxBytes),
			slog.Bool("in_use", e.lastAccess.After(now.Add(-p.ProtectWindow))))
	}
	b.oversize = still
}
