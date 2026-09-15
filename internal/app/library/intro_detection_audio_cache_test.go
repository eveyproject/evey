package library

import (
	"context"
	"errors"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// Invalidation contract of the Chromaprint cache: an entry is reused only
// when file fingerprint, algo version AND window all match; anything else is
// recomputed and written back, and a cache failure never blocks the compute.

type memAudioFPCache struct {
	entries  map[string]*domainlibrary.IntroAudioFingerprintCacheEntry
	getErr   error
	batchErr error
	upserts  int
}

func (c *memAudioFPCache) Get(_ context.Context, id string) (*domainlibrary.IntroAudioFingerprintCacheEntry, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}
	e, ok := c.entries[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return e, nil
}

func (c *memAudioFPCache) GetBatch(_ context.Context, ids []string) (map[string]*domainlibrary.IntroAudioFingerprintCacheEntry, error) {
	if c.batchErr != nil {
		return nil, c.batchErr
	}
	out := map[string]*domainlibrary.IntroAudioFingerprintCacheEntry{}
	for _, id := range ids {
		if e, ok := c.entries[id]; ok {
			out[id] = e
		}
	}
	return out, nil
}

func (c *memAudioFPCache) Upsert(_ context.Context, e *domainlibrary.IntroAudioFingerprintCacheEntry) error {
	c.upserts++
	if c.entries == nil {
		c.entries = map[string]*domainlibrary.IntroAudioFingerprintCacheEntry{}
	}
	c.entries[e.MediaItemID] = e
	return nil
}

func (c *memAudioFPCache) Delete(context.Context, string) error { return nil }
func (c *memAudioFPCache) DeleteByAlgoVersionBelow(context.Context, int16) (int64, error) {
	return 0, nil
}

func fingerprintedItem(id, fp string) *domainlibrary.MediaItem {
	return &domainlibrary.MediaItem{ID: shared.MediaItemID(id), File: &domainlibrary.MediaFile{Path: id, Fingerprint: fp}}
}

func validEntry(id, fp string, window int, data []uint32) *domainlibrary.IntroAudioFingerprintCacheEntry {
	return &domainlibrary.IntroAudioFingerprintCacheEntry{MediaItemID: id, FileFingerprint: fp, WindowSec: window, Fingerprint: data, AlgoVersion: currentAudioAlgoVersion}
}

func TestExtractEpisodeAudioCached_HitSkipsCompute(t *testing.T) {
	cache := &memAudioFPCache{entries: map[string]*domainlibrary.IntroAudioFingerprintCacheEntry{
		"e1": validEntry("e1", "fp-a", 300, []uint32{1, 2, 3}),
	}}
	cp := &pathChromaprinter{byPath: map[string][]uint32{"e1": {9, 9, 9}}}
	svc := audioConfirmService(t, cp)
	svc.WithAudioFingerprintCache(cache)

	got, err := svc.extractEpisodeAudioCached(context.Background(), fingerprintedItem("e1", "fp-a"), 300)
	if err != nil || len(got) != 3 || got[0] != 1 {
		t.Fatalf("got %v err=%v, want the cached fingerprint", got, err)
	}
	if cp.calls != 0 {
		t.Errorf("a valid entry must not trigger a compute, got %d calls", cp.calls)
	}
}

func TestExtractEpisodeAudioCached_StaleEntriesRecomputeAndWriteBack(t *testing.T) {
	stale := map[string]*domainlibrary.IntroAudioFingerprintCacheEntry{
		"file-changed": validEntry("file-changed", "fp-old", 300, []uint32{1}),
		"window":       validEntry("window", "fp-a", 120, []uint32{1}),
		"algo":         {MediaItemID: "algo", FileFingerprint: "fp-a", WindowSec: 300, Fingerprint: []uint32{1}, AlgoVersion: currentAudioAlgoVersion - 1},
	}
	for id := range stale {
		cache := &memAudioFPCache{entries: map[string]*domainlibrary.IntroAudioFingerprintCacheEntry{id: stale[id]}}
		cp := &pathChromaprinter{byPath: map[string][]uint32{id: {7, 7}}}
		svc := audioConfirmService(t, cp)
		svc.WithAudioFingerprintCache(cache)

		got, err := svc.extractEpisodeAudioCached(context.Background(), fingerprintedItem(id, "fp-a"), 300)
		if err != nil || len(got) != 2 || got[0] != 7 {
			t.Fatalf("%s: got %v err=%v, want a fresh compute", id, got, err)
		}
		if cp.calls != 1 || cache.upserts != 1 {
			t.Errorf("%s: compute=%d upserts=%d, want 1 and 1", id, cp.calls, cache.upserts)
		}
		if e := cache.entries[id]; e.FileFingerprint != "fp-a" || e.WindowSec != 300 || e.AlgoVersion != currentAudioAlgoVersion {
			t.Errorf("%s: written back entry = %+v", id, e)
		}
	}
}

func TestExtractEpisodeAudioCached_NoCacheOrNoFingerprintComputesDirectly(t *testing.T) {
	cp := &pathChromaprinter{byPath: map[string][]uint32{"e1": {5}}}
	svc := audioConfirmService(t, cp)
	// No cache wired.
	if got, err := svc.extractEpisodeAudioCached(context.Background(), fingerprintedItem("e1", "fp-a"), 300); err != nil || len(got) != 1 {
		t.Fatalf("no cache: got %v err=%v", got, err)
	}
	// Cache wired but the file has no fingerprint yet: the cache is bypassed
	// both ways.
	cache := &memAudioFPCache{}
	svc.WithAudioFingerprintCache(cache)
	if got, err := svc.extractEpisodeAudioCached(context.Background(), fingerprintedItem("e1", ""), 300); err != nil || len(got) != 1 {
		t.Fatalf("no file fingerprint: got %v err=%v", got, err)
	}
	if cache.upserts != 0 {
		t.Errorf("a file without fingerprint must not be cached, got %d upserts", cache.upserts)
	}
}

func TestExtractEpisodeAudioCached_ComputeFailureIsNotCached(t *testing.T) {
	cache := &memAudioFPCache{}
	cp := &pathChromaprinter{failOn: map[string]bool{"e1": true}}
	svc := audioConfirmService(t, cp)
	svc.WithAudioFingerprintCache(cache)
	if _, err := svc.extractEpisodeAudioCached(context.Background(), fingerprintedItem("e1", "fp-a"), 300); err == nil {
		t.Fatal("expected the compute error")
	}
	if cache.upserts != 0 {
		t.Errorf("a failed compute must not be written back, got %d upserts", cache.upserts)
	}
}

func TestPrefetchAudioFingerprints_KeepsOnlyValidEntries(t *testing.T) {
	cache := &memAudioFPCache{entries: map[string]*domainlibrary.IntroAudioFingerprintCacheEntry{
		"ok":      validEntry("ok", "fp-a", 300, []uint32{1}),
		"stale":   validEntry("stale", "fp-old", 300, []uint32{1}),
		"window":  validEntry("window", "fp-a", 120, []uint32{1}),
		"empty":   validEntry("empty", "fp-a", 300, nil),
		"algo":    {MediaItemID: "algo", FileFingerprint: "fp-a", WindowSec: 300, Fingerprint: []uint32{1}, AlgoVersion: currentAudioAlgoVersion - 1},
		"unasked": validEntry("unasked", "fp-a", 300, []uint32{1}),
	}}
	svc := audioConfirmService(t, &pathChromaprinter{})
	svc.WithAudioFingerprintCache(cache)
	items := []*domainlibrary.MediaItem{
		fingerprintedItem("ok", "fp-a"), fingerprintedItem("stale", "fp-a"), fingerprintedItem("window", "fp-a"),
		fingerprintedItem("empty", "fp-a"), fingerprintedItem("algo", "fp-a"),
		fingerprintedItem("nofp", ""), {ID: "nofile"}, fingerprintedItem("miss", "fp-a"),
	}
	got := svc.prefetchAudioFingerprints(context.Background(), items, 300)
	if len(got) != 1 || len(got["ok"]) != 1 {
		t.Fatalf("got %v, want only the valid entry", got)
	}
}

func TestPrefetchAudioFingerprints_DegradesToEmpty(t *testing.T) {
	svc := audioConfirmService(t, &pathChromaprinter{})
	items := []*domainlibrary.MediaItem{fingerprintedItem("e1", "fp-a")}
	// No cache: an empty, non-nil map so callers can index it.
	if got := svc.prefetchAudioFingerprints(context.Background(), items, 300); got == nil || len(got) != 0 {
		t.Errorf("no cache: got %v, want empty map", got)
	}
	// A failing batch read is a cache miss for everyone, never an error.
	svc.WithAudioFingerprintCache(&memAudioFPCache{batchErr: errors.New("db down")})
	if got := svc.prefetchAudioFingerprints(context.Background(), items, 300); got == nil || len(got) != 0 {
		t.Errorf("batch error: got %v, want empty map", got)
	}
	if got := svc.prefetchAudioFingerprints(context.Background(), nil, 300); got == nil || len(got) != 0 {
		t.Errorf("no items: got %v, want empty map", got)
	}
}
