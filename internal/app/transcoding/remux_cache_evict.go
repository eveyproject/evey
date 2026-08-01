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
	"time"
)

// DefaultRemuxCacheBytes caps the on-disk fast-start MP4 remux cache
// (<CacheDir>/remux). Fixed at 10 GiB: a cache budget is rarely worth an
// operator knob, and a sensible bound beats an env var (settings belong in the
// admin panel, not .env). Swept independently of the segment cache.
const DefaultRemuxCacheBytes = 10 * 1024 * 1024 * 1024

// DefaultCompatCacheBytes caps the compat-download cache (<CacheDir>/compat).
// Entries are whole films (video copied, audio re-encoded), so a handful of
// titles already reaches tens of GiB; 100 GiB keeps roughly a household's
// worth of recently-fetched films warm without an operator knob.
const DefaultCompatCacheBytes = 100 * 1024 * 1024 * 1024

// RemuxCacheJanitor bounds a flat-file cache directory - the fast-start MP4s the
// /remux.mp4 handler writes under <CacheDir>/remux, one whole file per entry. It
// reuses the segment cache's EvictionPolicy knobs and LRU-by-(size, mtime)
// algorithm, but operates on FILES, not the segment cache's per-decision-hash
// directories with session refcounts (a remuxed file has no live session to
// protect, so cleanupHashIfIdle / pendingRefs don't apply). Source-fingerprint
// keying means every changed source orphans its old file here, so this janitor
// is what keeps that from growing without bound.
//
// LastAccess is the file mtime, which a remuxed file freezes at production time
// (it's written once, never appended). That makes eviction "least-recently-
// PRODUCED" rather than "least-recently-served": a popular but old remux can be
// evicted and re-produced on next request. That re-remux is a cheap copy (no
// re-encode) and only happens above MaxBytes, so it's an acceptable first cut;
// an in-memory access registry (as SegmentCache keeps) is the follow-up to keep
// hot files warm.
type RemuxCacheJanitor struct {
	dir string
}

// NewRemuxCacheJanitor sweeps dir (typically <CacheDir>/remux).
func NewRemuxCacheJanitor(dir string) *RemuxCacheJanitor {
	return &RemuxCacheJanitor{dir: dir}
}

// StartEviction launches the background sweeper. MaxBytes <= 0 disables it and
// returns a no-op stop (unbounded growth - only sane for tests/POC). The stop
// function is safe to defer.
func (j *RemuxCacheJanitor) StartEviction(policy EvictionPolicy, logger *slog.Logger) (stop func()) {
	policy = policy.withDefaults()
	if policy.MaxBytes <= 0 {
		return func() {}
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "remux_cache_evict")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(policy.SweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// A panic in one sweep must not crash the whole server (weak-NAS,
				// multi-user) nor stop future sweeps - recover per iteration, like
				// every sibling background worker.
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("panic in remux cache sweep", slog.Any("panic", r))
						}
					}()
					j.sweepOnce(policy, logger)
				}()
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// sweepOnce totals the .mp4 files in the dir and, if above MaxBytes, deletes
// them in least-recently-modified order until usage drops below
// MaxBytes - MarginBytes. The N freshest files are protected (ProtectRecent).
func (j *RemuxCacheJanitor) sweepOnce(p EvictionPolicy, logger *slog.Logger) {
	files, err := j.listFiles()
	if err != nil {
		logger.Warn("remux sweep: list failed", slog.Any("err", err))
		return
	}
	if len(files) == 0 {
		return
	}
	var total int64
	for _, f := range files {
		total += f.bytes
	}
	if total <= p.MaxBytes {
		return
	}

	sort.Slice(files, func(a, b int) bool { return files[a].mtime.Before(files[b].mtime) })
	protected := min(p.ProtectRecent, len(files))
	candidates := files[:len(files)-protected]

	target := max(p.MaxBytes-p.MarginBytes, 0)

	var evicted int
	var freed int64
	for _, f := range candidates {
		if total <= target {
			break
		}
		if err := os.Remove(f.path); err != nil {
			logger.Warn("remux sweep: remove failed", slog.String("path", f.path), slog.Any("err", err))
			continue
		}
		total -= f.bytes
		freed += f.bytes
		evicted++
	}
	if evicted > 0 {
		logger.Info("remux sweep: evicted",
			slog.Int("files", evicted),
			slog.Int64("freed_bytes", freed),
			slog.Int64("total_after_bytes", total),
			slog.Int("protected", protected))
	}
}

type remuxFileUsage struct {
	path  string
	bytes int64
	mtime time.Time
}

// listFiles enumerates completed remux outputs (*.mp4) one level deep. In-flight
// "*.mp4.tmp" files are skipped so a sweep racing a producer never deletes a
// half-written file out from under it.
func (j *RemuxCacheJanitor) listFiles() ([]remuxFileUsage, error) {
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("remux listFiles: %w", err)
	}
	var out []remuxFileUsage
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".mp4" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, remuxFileUsage{
			path:  filepath.Join(j.dir, e.Name()),
			bytes: info.Size(),
			mtime: info.ModTime(),
		})
	}
	return out, nil
}
