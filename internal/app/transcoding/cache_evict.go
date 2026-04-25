package transcoding

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// Watermarks for the LRU evictor.
//
// We start evicting once usage crosses HighWatermark (95%) of the configured
// cap and stop once we've fallen back to LowWatermark (80%). The hysteresis
// avoids re-triggering eviction every tick when usage hovers around the cap.
const (
	highWatermarkPct = 0.95
	lowWatermarkPct  = 0.80

	defaultEvictInterval = 10 * time.Minute
)

// clock is a minimal time abstraction local to the transcoding package so
// tests can drive the ticker deterministically. It is intentionally not
// shared with other packages — another agent owns the library-level Clock.
type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// fileSystem abstracts the few filesystem ops the evictor needs so tests
// can inject failures (e.g. a permission-denied subdir) without juggling
// real OS permissions.
type fileSystem interface {
	ReadDir(path string) ([]os.DirEntry, error)
	WalkDir(root string, fn fs.WalkDirFunc) error
	RemoveAll(path string) error
}

type osFS struct{}

func (osFS) ReadDir(path string) ([]os.DirEntry, error)         { return os.ReadDir(path) }
func (osFS) WalkDir(root string, fn fs.WalkDirFunc) error       { return filepath.WalkDir(root, fn) }
func (osFS) RemoveAll(path string) error                        { return os.RemoveAll(path) }

// CacheEvictor periodically scans the transcode cache and removes the
// least-recently-used session directories when total disk usage crosses
// the high watermark. It is independent of the in-flight session map
// owned by HLSManager — this is a janitor for orphaned/cold cache, not
// a replacement for HLSManager.cleanupStale (which evicts hot sessions
// based on idle time).
type CacheEvictor struct {
	cacheRoot string
	maxBytes  int64
	interval  time.Duration
	clock     clock
	fs        fileSystem
}

// Option mutates CacheEvictor construction.
type EvictorOption func(*CacheEvictor)

// WithClock injects a clock implementation (tests).
func WithClock(c clock) EvictorOption {
	return func(e *CacheEvictor) { e.clock = c }
}

// WithFileSystem injects a filesystem implementation (tests).
func WithFileSystem(f fileSystem) EvictorOption {
	return func(e *CacheEvictor) { e.fs = f }
}

// NewCacheEvictor builds an evictor for cacheRoot capped at maxGB.
// maxGB <= 0 disables the evictor (Run returns immediately, EvictOnce is a no-op).
// interval <= 0 falls back to defaultEvictInterval.
func NewCacheEvictor(cacheRoot string, maxGB float64, interval time.Duration, opts ...EvictorOption) *CacheEvictor {
	var maxBytes int64
	if maxGB > 0 {
		maxBytes = int64(maxGB * 1024 * 1024 * 1024)
	}
	if interval <= 0 {
		interval = defaultEvictInterval
	}
	e := &CacheEvictor{
		cacheRoot: cacheRoot,
		maxBytes:  maxBytes,
		interval:  interval,
		clock:     realClock{},
		fs:        osFS{},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run drives a periodic eviction loop until ctx is cancelled. Disabled
// evictors (maxBytes == 0) log once and return — this lets the caller
// always Go-launch the evictor without branching on the config value.
func (e *CacheEvictor) Run(ctx context.Context) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.CacheEvictor.Run")
	defer span.End()
	span.SetAttributes(attribute.String("cache.root", e.cacheRoot))
	if e.maxBytes == 0 {
		slog.Info("transcode cache evictor disabled (maxGB <= 0)", slog.String("cache_root", e.cacheRoot))
		return
	}
	slog.Info("transcode cache evictor started",
		slog.String("cache_root", e.cacheRoot),
		slog.Int64("max_bytes", e.maxBytes),
		slog.Duration("interval", e.interval),
	)
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("transcode cache evictor stopped", slog.Any("err", ctx.Err()))
			return
		case <-ticker.C:
			if freed, err := e.EvictOnce(ctx); err != nil {
				slog.Warn("transcode cache evict pass failed", slog.Any("err", err))
			} else if freed > 0 {
				slog.Info("transcode cache evicted",
					slog.Int64("bytes_freed", freed),
					slog.String("cache_root", e.cacheRoot),
				)
			}
		}
	}
}

// EvictOnce performs a single eviction pass and returns the number of
// bytes freed. Safe to call from admin endpoints for manual triggers.
func (e *CacheEvictor) EvictOnce(ctx context.Context) (freed int64, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.CacheEvictor.EvictOnce")
	defer span.End()
	span.SetAttributes(attribute.String("cache.root", e.cacheRoot))
	defer func() {
		span.SetAttributes(attribute.Int64("bytes.freed", freed))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
		}
	}()
	if e.maxBytes == 0 {
		return 0, nil
	}

	entries, err := e.fs.ReadDir(e.cacheRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	type subdir struct {
		path       string
		size       int64
		lastAccess time.Time
	}

	var subdirs []subdir
	var total int64
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Join(e.cacheRoot, ent.Name())
		size, latest, walkErr := e.statSubdir(path)
		if walkErr != nil {
			// Already logged in statSubdir — keep going so a single bad
			// subdir doesn't paralyse eviction for the whole cache.
			continue
		}
		subdirs = append(subdirs, subdir{path: path, size: size, lastAccess: latest})
		total += size
	}

	highMark := int64(float64(e.maxBytes) * highWatermarkPct)
	if total <= highMark {
		return 0, nil
	}

	// Oldest LRU first.
	sort.Slice(subdirs, func(i, j int) bool {
		return subdirs[i].lastAccess.Before(subdirs[j].lastAccess)
	})

	lowMark := int64(float64(e.maxBytes) * lowWatermarkPct)
	for _, s := range subdirs {
		if total <= lowMark {
			break
		}
		if ctx.Err() != nil {
			return freed, ctx.Err()
		}
		if err := e.fs.RemoveAll(s.path); err != nil {
			slog.Warn("transcode cache: remove subdir failed",
				slog.String("path", s.path),
				slog.Any("err", err),
			)
			continue
		}
		freed += s.size
		total -= s.size
		slog.Debug("transcode cache: evicted lru subdir",
			slog.String("path", s.path),
			slog.Int64("size", s.size),
			slog.Time("last_access", s.lastAccess),
		)
	}
	return freed, nil
}

// statSubdir walks a single cache subdirectory and returns its total size
// plus the most recent file modtime (proxy for last access — most Linux
// filesystems mount with `noatime` so atime is unreliable). Permission
// errors are logged + skipped so one locked subdir doesn't kill the pass.
func (e *CacheEvictor) statSubdir(root string) (int64, time.Time, error) {
	var total int64
	var latest time.Time
	hadError := false

	walkErr := e.fs.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("transcode cache: walk error",
				slog.String("path", path),
				slog.Any("err", err),
			)
			hadError = true
			// Skip the offending entry but keep walking siblings.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			slog.Warn("transcode cache: stat error",
				slog.String("path", path),
				slog.Any("err", err),
			)
			hadError = true
			return nil
		}
		total += fi.Size()
		if fi.ModTime().After(latest) {
			latest = fi.ModTime()
		}
		return nil
	})
	if walkErr != nil {
		return 0, time.Time{}, walkErr
	}
	if hadError && total == 0 {
		// Nothing readable — surface so caller skips this subdir entirely.
		return 0, time.Time{}, fs.ErrPermission
	}
	return total, latest, nil
}
