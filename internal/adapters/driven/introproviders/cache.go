package introproviders

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// CacheRepo is the read/write port for the postgres-backed cache. The
// repo lives in internal/adapters/driven/postgres but the cache
// decorator is here so it can stay private to the introproviders
// package and not leak into the postgres adapters' public surface.
type CacheRepo interface {
	// Get returns the cached segments and the time they were stored, or
	// (nil, zero, nil) on a cache miss - the decorator treats that as a
	// miss and hits the upstream.
	Get(ctx context.Context, tmdbID, season, episode int) ([]byte, time.Time, error)
	// Put stores the JSON payload. season/episode = 0 for movies.
	// `source` is the chain decision (which provider answered) so the
	// admin UI can audit / invalidate per-source.
	Put(ctx context.Context, tmdbID, season, episode int, source string, payload []byte) error
}

// Cached wraps an inner provider with a postgres lookup. On hit, the
// inner provider is never called - this is the volume-control knob
// that keeps us under the daily TIDB quota for typical scans.
//
// Negative caching: a (200 + empty) or 404 response also gets cached as
// `[]` so we don't keep hammering upstream for items neither DB knows.
// negativeTTL controls how long a negative row blocks upstream retries;
// 0 means "forever" (legacy / explicit no-refresh).
type Cached struct {
	inner       domainlibrary.IntroMetadataProvider
	resolver    *AncestorResolver
	repo        CacheRepo
	negativeTTL time.Duration    // 0 = forever
	nowFunc     func() time.Time // injectable for tests
}

// NewCached wraps inner with a postgres-backed cache. resolver is reused
// so we don't walk ancestors twice per call.
func NewCached(inner domainlibrary.IntroMetadataProvider, resolver *AncestorResolver, repo CacheRepo) *Cached {
	return &Cached{inner: inner, resolver: resolver, repo: repo, nowFunc: time.Now}
}

// WithNegativeTTL sets the maximum age of a negative (empty) cache row
// before it is treated as a miss and the upstream is retried. This lets
// newly-added TIDB data surface without a manual cache invalidation.
// A value of 0 keeps the legacy behaviour (negative rows never expire).
func (c *Cached) WithNegativeTTL(d time.Duration) *Cached {
	c.negativeTTL = d
	return c
}

// GetSegments first probes the cache; on miss, delegates to inner and
// persists whatever came back (including empty results, see negative
// caching note above).
func (c *Cached) GetSegments(ctx context.Context, item *domainlibrary.MediaItem) ([]domainlibrary.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "introproviders.cache.GetSegments")
	defer span.End()

	lookup, err := c.resolver.Resolve(ctx, item)
	if err != nil {
		if errors.Is(err, ErrLookupUnresolvable) {
			return nil, nil
		}
		// Don't fail the pipeline on cache resolve errors - let the
		// inner provider try (it will resolve again).
		return c.inner.GetSegments(ctx, item)
	}

	if c.repo != nil {
		if blob, fetchedAt, getErr := c.repo.Get(ctx, lookup.TmdbID, lookup.Season, lookup.Episode); getErr == nil && blob != nil {
			isNegative := string(blob) == "[]"
			if isNegative && c.negativeTTL > 0 && c.nowFunc().Sub(fetchedAt) > c.negativeTTL {
				// Negative cache row is stale - treat as miss so the
				// upstream is retried (TIDB may have new data now).
				slog.Debug("intro_metadata_cache: negative row expired, refetching",
					slog.Int("tmdb_id", lookup.TmdbID),
					slog.Duration("age", c.nowFunc().Sub(fetchedAt)),
					slog.Duration("ttl", c.negativeTTL))
			} else {
				var cached []domainlibrary.MediaSegment
				if jerr := json.Unmarshal(blob, &cached); jerr == nil {
					return cached, nil
				}
				slog.Warn("intro_metadata_cache: bad json in cache row, refetching",
					slog.Int("tmdb_id", lookup.TmdbID))
			}
		}
	}

	segs, err := c.inner.GetSegments(ctx, item)
	if err != nil {
		// Don't cache transient errors - the next call should retry.
		return nil, err
	}

	if c.repo != nil {
		// Empty slice serialises to "[]" - also a valid cache row.
		blob, jerr := json.Marshal(segs)
		if jerr == nil {
			source := "chain"
			if len(segs) > 0 {
				source = segs[0].Source
			}
			if putErr := c.repo.Put(ctx, lookup.TmdbID, lookup.Season, lookup.Episode, source, blob); putErr != nil {
				slog.Warn("intro_metadata_cache: put failed",
					slog.Int("tmdb_id", lookup.TmdbID), slog.Any("err", putErr))
			}
		}
	}
	return segs, nil
}
