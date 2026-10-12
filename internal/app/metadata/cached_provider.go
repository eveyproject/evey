package metadata

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

// CachedProvider memoizes Search and GetByProviderID responses for a TTL.
// Cache keys are derived from (provider name, args). Only successful
// responses are cached; errors always hit the network.
//
// Intent: avoid re-querying the same title during back-to-back scans or
// admin refresh operations. Modest hit-rate expected in practice, but
// zero-cost when cold and cheap when warm.
type CachedProvider struct {
	inner domainmetadata.MetadataProvider
	ttl   time.Duration

	mu           sync.RWMutex
	searchStore  map[string]searchEntry
	getByIDStore map[string]getByIDEntry
}

type searchEntry struct {
	expires time.Time
	results []domainmetadata.MetadataResult
}

type getByIDEntry struct {
	expires time.Time
	result  *domainmetadata.MetadataResult
}

// maxCacheEntries caps each store. The TTL is only checked on READ (it hides
// expired entries, it never frees them), so without a cap a long scan or a
// flood of distinct free-text searches - both feed these maps, via the
// enricher and the interactive MetadataService - would grow them until restart
// and eventually OOM a small box. On overflow we reclaim expired entries first,
// then evict down to 3/4 of the cap (headroom so we don't re-prune every put),
// keeping the back-to-back scan/refresh dedup the cache is for.
const maxCacheEntries = 2048

// pruneAndCap reclaims expired entries from m, then, if still at/over the cap,
// evicts arbitrary entries until len(m) < target. Caller holds the write lock.
func pruneAndCap[V any](m map[string]V, expiresOf func(V) time.Time, now time.Time, target int) {
	for k, v := range m {
		if now.After(expiresOf(v)) {
			delete(m, k)
		}
	}
	for k := range m {
		if len(m) < target {
			break
		}
		delete(m, k)
	}
}

// NewCachedProvider wraps inner with a TTL cache. A non-positive ttl
// disables caching (useful in tests).
func NewCachedProvider(inner domainmetadata.MetadataProvider, ttl time.Duration) *CachedProvider {
	return &CachedProvider{
		inner:        inner,
		ttl:          ttl,
		searchStore:  make(map[string]searchEntry),
		getByIDStore: make(map[string]getByIDEntry),
	}
}

// lang returns the inner provider's current language for cache-key folding, or
// "" when the provider's results don't depend on a language.
func (c *CachedProvider) lang() string {
	if la, ok := c.inner.(domainmetadata.LanguageAware); ok {
		return la.Language()
	}
	return ""
}

func (c *CachedProvider) Name() string                            { return c.inner.Name() }
func (c *CachedProvider) Unwrap() domainmetadata.MetadataProvider { return c.inner }

func (c *CachedProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domainmetadata.MetadataResult, error) {
	if c.ttl <= 0 {
		return c.inner.Search(ctx, query, year, mediaType)
	}
	key := searchKey(c.inner.Name(), c.lang(), query, year, mediaType)
	if results, ok := c.getSearch(key); ok {
		return results, nil
	}
	results, err := c.inner.Search(ctx, query, year, mediaType)
	if err != nil {
		return results, err
	}
	c.putSearch(key, results)
	return results, nil
}

func (c *CachedProvider) GetByProviderID(ctx context.Context, providerID, mediaType string) (*domainmetadata.MetadataResult, error) {
	if c.ttl <= 0 {
		return c.inner.GetByProviderID(ctx, providerID, mediaType)
	}
	key := fmt.Sprintf("%s|%s|%s|%s", c.inner.Name(), c.lang(), providerID, mediaType)
	if result, ok := c.getByID(key); ok {
		return result, nil
	}
	result, err := c.inner.GetByProviderID(ctx, providerID, mediaType)
	if err != nil {
		return result, err
	}
	c.putByID(key, result)
	return result, nil
}

func (c *CachedProvider) getSearch(key string) ([]domainmetadata.MetadataResult, bool) {
	c.mu.RLock()
	entry, ok := c.searchStore[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.results, true
}

func (c *CachedProvider) putSearch(key string, results []domainmetadata.MetadataResult) {
	now := time.Now()
	c.mu.Lock()
	if len(c.searchStore) >= maxCacheEntries {
		pruneAndCap(c.searchStore, func(e searchEntry) time.Time { return e.expires }, now, maxCacheEntries*3/4)
	}
	c.searchStore[key] = searchEntry{expires: now.Add(c.ttl), results: results}
	c.mu.Unlock()
}

func (c *CachedProvider) getByID(key string) (*domainmetadata.MetadataResult, bool) {
	c.mu.RLock()
	entry, ok := c.getByIDStore[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.result, true
}

func (c *CachedProvider) putByID(key string, result *domainmetadata.MetadataResult) {
	now := time.Now()
	c.mu.Lock()
	if len(c.getByIDStore) >= maxCacheEntries {
		pruneAndCap(c.getByIDStore, func(e getByIDEntry) time.Time { return e.expires }, now, maxCacheEntries*3/4)
	}
	c.getByIDStore[key] = getByIDEntry{expires: now.Add(c.ttl), result: result}
	c.mu.Unlock()
}

func searchKey(name, lang, query string, year *int, mediaType string) string {
	y := ""
	if year != nil {
		y = strconv.Itoa(*year)
	}
	return name + "|s|" + lang + "|" + query + "|" + y + "|" + mediaType
}
