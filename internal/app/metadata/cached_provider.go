package metadata

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
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

func (c *CachedProvider) Name() string                        { return c.inner.Name() }
func (c *CachedProvider) Unwrap() domainmetadata.MetadataProvider { return c.inner }

func (c *CachedProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domainmetadata.MetadataResult, error) {
	if c.ttl <= 0 {
		return c.inner.Search(ctx, query, year, mediaType)
	}
	key := searchKey(c.inner.Name(), query, year, mediaType)
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
	key := fmt.Sprintf("%s|%s|%s", c.inner.Name(), providerID, mediaType)
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
	c.mu.Lock()
	c.searchStore[key] = searchEntry{expires: time.Now().Add(c.ttl), results: results}
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
	c.mu.Lock()
	c.getByIDStore[key] = getByIDEntry{expires: time.Now().Add(c.ttl), result: result}
	c.mu.Unlock()
}

func searchKey(name, query string, year *int, mediaType string) string {
	y := ""
	if year != nil {
		y = strconv.Itoa(*year)
	}
	return name + "|s|" + query + "|" + y + "|" + mediaType
}
