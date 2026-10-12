package metadata

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

func TestCachedProvider_SearchMemoizesSuccess(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:         "tmdb",
		searchResult: []domainmetadata.MetadataResult{{Title: "hit"}},
	}
	c := NewCachedProvider(inner, time.Minute)
	y := 2020
	for i := 0; i < 3; i++ {
		got, err := c.Search(context.Background(), "some movie", &y, "movie")
		if err != nil || len(got) != 1 {
			t.Fatalf("iter %d: err=%v got=%v", i, err, got)
		}
	}
	if inner.searchCalls != 1 {
		t.Fatalf("inner.searchCalls = %d, want 1 (cached)", inner.searchCalls)
	}
}

func TestCachedProvider_SearchKeyDiscriminatesArgs(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:         "tmdb",
		searchResult: []domainmetadata.MetadataResult{{Title: "x"}},
	}
	c := NewCachedProvider(inner, time.Minute)
	y1, y2 := 2020, 2021
	_, _ = c.Search(context.Background(), "some movie", &y1, "movie")
	_, _ = c.Search(context.Background(), "some movie", &y2, "movie")
	_, _ = c.Search(context.Background(), "some movie", nil, "movie")
	_, _ = c.Search(context.Background(), "some movie", &y1, "tv")
	if inner.searchCalls != 4 {
		t.Fatalf("inner.searchCalls = %d, want 4 (distinct keys)", inner.searchCalls)
	}
}

func TestCachedProvider_DoesNotCacheErrors(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:       "omdb",
		searchErrs: []error{errors.New("boom"), errors.New("boom")},
	}
	c := NewCachedProvider(inner, time.Minute)
	if _, err := c.Search(context.Background(), "q", nil, "movie"); err == nil {
		t.Fatal("expected err")
	}
	if _, err := c.Search(context.Background(), "q", nil, "movie"); err == nil {
		t.Fatal("expected err")
	}
	if inner.searchCalls != 2 {
		t.Fatalf("inner.searchCalls = %d, want 2 (errors not cached)", inner.searchCalls)
	}
}

func TestCachedProvider_Expiration(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:         "tmdb",
		searchResult: []domainmetadata.MetadataResult{{Title: "x"}},
	}
	c := NewCachedProvider(inner, 5*time.Millisecond)
	_, _ = c.Search(context.Background(), "q", nil, "movie")
	_, _ = c.Search(context.Background(), "q", nil, "movie")
	if inner.searchCalls != 1 {
		t.Fatalf("pre-expiry calls = %d, want 1", inner.searchCalls)
	}
	time.Sleep(10 * time.Millisecond)
	_, _ = c.Search(context.Background(), "q", nil, "movie")
	if inner.searchCalls != 2 {
		t.Fatalf("post-expiry calls = %d, want 2", inner.searchCalls)
	}
}

func TestCachedProvider_ZeroTTLDisablesCache(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:         "tmdb",
		searchResult: []domainmetadata.MetadataResult{{Title: "x"}},
	}
	c := NewCachedProvider(inner, 0)
	for i := 0; i < 3; i++ {
		_, _ = c.Search(context.Background(), "q", nil, "movie")
	}
	if inner.searchCalls != 3 {
		t.Fatalf("calls = %d, want 3 (ttl=0 bypasses cache)", inner.searchCalls)
	}
}

func TestCachedProvider_GetByProviderIDMemoizes(t *testing.T) {
	result := &domainmetadata.MetadataResult{Title: "hit"}
	inner := &decoratorFakeProvider{name: "tmdb", idResult: result}
	c := NewCachedProvider(inner, time.Minute)
	for i := 0; i < 3; i++ {
		got, err := c.GetByProviderID(context.Background(), "42", "movie")
		if err != nil || got != result {
			t.Fatalf("iter %d: err=%v got=%v", i, err, got)
		}
	}
	if inner.getByIDCalls != 1 {
		t.Fatalf("calls = %d, want 1", inner.getByIDCalls)
	}
}

func TestCachedProvider_StoresAreBounded(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:         "tmdb",
		searchResult: []domainmetadata.MetadataResult{{Title: "x"}},
		idResult:     &domainmetadata.MetadataResult{Title: "y"},
	}
	// Long TTL → nothing expires, so only the hard cap can keep the maps from
	// growing one entry per distinct query/id forever (the OOM the audit found).
	c := NewCachedProvider(inner, time.Hour)
	for i := range maxCacheEntries + 1024 {
		_, _ = c.Search(context.Background(), "q"+strconv.Itoa(i), nil, "movie")
		_, _ = c.GetByProviderID(context.Background(), strconv.Itoa(i), "movie")
	}
	c.mu.RLock()
	ns, ng := len(c.searchStore), len(c.getByIDStore)
	c.mu.RUnlock()
	if ns > maxCacheEntries {
		t.Fatalf("searchStore grew to %d entries, exceeds cap %d (unbounded growth)", ns, maxCacheEntries)
	}
	if ng > maxCacheEntries {
		t.Fatalf("getByIDStore grew to %d entries, exceeds cap %d (unbounded growth)", ng, maxCacheEntries)
	}
}

func TestCachedProvider_Unwrap(t *testing.T) {
	inner := &decoratorFakeProvider{name: "tmdb"}
	c := NewCachedProvider(inner, time.Minute)
	if c.Unwrap() != inner {
		t.Fatal("Unwrap must return inner")
	}
	if c.Name() != "tmdb" {
		t.Fatalf("name = %q", c.Name())
	}
}

func TestCachedProvider_StackedWithRetry_UnwrapReachesBase(t *testing.T) {
	inner := &decoratorFakeProvider{name: "tmdb"}
	stacked := NewCachedProvider(NewRetryProvider(inner, 3, time.Microsecond), time.Minute)
	cur := domainmetadata.MetadataProvider(stacked)
	for {
		u, ok := cur.(domainmetadata.ProviderUnwrapper)
		if !ok {
			break
		}
		cur = u.Unwrap()
	}
	if cur != inner {
		t.Fatalf("unwrap chain did not reach inner: got %T", cur)
	}
}

// langAwareFake adds a settable Language() so the cache key folds it in.
type langAwareFake struct {
	*decoratorFakeProvider
	lang string
}

func (f *langAwareFake) Language() string { return f.lang }

// A language change must not serve a result fetched under another language -
// otherwise a synopsis fetched in one language sticks under another.
func TestCachedProvider_SearchKeyDiscriminatesLanguage(t *testing.T) {
	inner := &langAwareFake{
		decoratorFakeProvider: &decoratorFakeProvider{name: "tmdb", searchResult: []domainmetadata.MetadataResult{{Title: "x"}}},
		lang:                  "en",
	}
	c := NewCachedProvider(inner, time.Minute)

	_, _ = c.Search(context.Background(), "some movie", nil, "movie") // miss → fetch (en)
	_, _ = c.Search(context.Background(), "some movie", nil, "movie") // hit (en)
	if inner.searchCalls != 1 {
		t.Fatalf("after same-language repeat: searchCalls=%d, want 1", inner.searchCalls)
	}

	inner.lang = "fr"
	_, _ = c.Search(context.Background(), "some movie", nil, "movie") // miss → fetch (fr)
	if inner.searchCalls != 2 {
		t.Fatalf("after language switch: searchCalls=%d, want 2 (not served from en cache)", inner.searchCalls)
	}
}

func TestCachedProvider_GetByIDKeyDiscriminatesLanguage(t *testing.T) {
	inner := &langAwareFake{
		decoratorFakeProvider: &decoratorFakeProvider{name: "tmdb", idResult: &domainmetadata.MetadataResult{Title: "x"}},
		lang:                  "en",
	}
	c := NewCachedProvider(inner, time.Minute)

	_, _ = c.GetByProviderID(context.Background(), "9772", "movie")
	_, _ = c.GetByProviderID(context.Background(), "9772", "movie")
	if inner.getByIDCalls != 1 {
		t.Fatalf("same-language repeat: getByIDCalls=%d, want 1", inner.getByIDCalls)
	}
	inner.lang = "fr"
	_, _ = c.GetByProviderID(context.Background(), "9772", "movie")
	if inner.getByIDCalls != 2 {
		t.Fatalf("language switch: getByIDCalls=%d, want 2", inner.getByIDCalls)
	}
}
