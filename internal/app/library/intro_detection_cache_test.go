package library

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	libraryfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// hashesDetector returns the configured tileHashes from ExtractTileHashes
// for spritePaths ending in trickplay_001.webp, and an error otherwise - so
// extractEpisodeHashes' loop terminates after sprite 1 (mimicking
// "subsequent sprites don't exist on disk"). When the cache short-circuits
// the second logical call, no sprite read happens at all and extractCalls
// stays at the count from the first miss.
type hashesDetector struct {
	tileHashes   []uint64
	extractCalls int
}

func (d *hashesDetector) ExtractTileHashes(spritePath string, _, _, _, _ int) ([]uint64, error) {
	d.extractCalls++
	// Only the first sprite exists; subsequent indices terminate the loop.
	if !endsWith(spritePath, "trickplay_001.webp") {
		return nil, errors.New("sprite not found")
	}
	return d.tileHashes, nil
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
func (d *hashesDetector) PrecisionPass(_ context.Context, _ string, s, e, _ float64) (int64, int64, error) {
	return int64(s * 1000), int64(e * 1000), nil
}
func (d *hashesDetector) AudioFallback(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (d *hashesDetector) DetectEndCredits(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (d *hashesDetector) DetectStudioLogo(_ []uint64, _ int) (int64, int64, bool) {
	return 0, 0, false
}
func (d *hashesDetector) DetectLeadingLogoBlock(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (d *hashesDetector) AudioBandedFingerprint(_ context.Context, _ string, _, _ float64, _ int) ([]float64, error) {
	return nil, nil
}
func (d *hashesDetector) SubtitleCueStarts(_ context.Context, _ string) ([]int64, error) {
	return nil, nil
}
func (d *hashesDetector) SceneCutTimestamps(_ context.Context, _ string, _, _, _ float64) ([]int64, error) {
	return nil, nil
}
func (d *hashesDetector) GrayThumbnails(_ context.Context, _ string, _, _, _ float64) ([][]byte, error) {
	return nil, nil
}

// newServiceWithCache wires the smallest possible IntroDetectionService for
// cache-behavior tests: a detector that fails on second extract, a temp
// trickplay cache dir + sidecar with N frames so extractEpisodeHashes can
// run on first miss, and a fake fingerprint cache.
func newServiceWithCache(t *testing.T, det IntroDetector, cache domainlibrary.IntroFingerprintCacheRepository) (*IntroDetectionService, *domainlibrary.TrickplayManifest) {
	t.Helper()
	manifest := &domainlibrary.TrickplayManifest{
		MediaItemID: shared.MediaItemID("media-1"),
		Interval:    7,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  3,
		ImageCount:  1,
		ImagePath:   "",
	}
	cacheDir := t.TempDir()
	totalFrames := manifest.ImageCount * manifest.TileWidth * manifest.TileHeight
	writeTrickplaySidecarFixture(t, cacheDir, string(manifest.MediaItemID), manifest.Interval, manifest.TileWidth*manifest.TileHeight, totalFrames)
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepoWithManifest{manifest: manifest},
		&fakeMediaItemRepo{},
		&fakeConfigRepo{},
		det,
		cacheDir,
	)
	if cache != nil {
		svc.WithFingerprintCache(cache)
	}
	return svc, manifest
}

func TestExtractEpisodeHashesCached_NilCache_AlwaysExtracts(t *testing.T) {
	t.Parallel()
	det := &hashesDetector{tileHashes: []uint64{1, 2, 3}}
	svc, manifest := newServiceWithCache(t, det, nil)
	cutoff := manifestTotalFrames(manifest)

	hashes, _, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "fp-A", manifest, cutoff)
	require.NoError(t, err)
	require.NotEmpty(t, hashes)
	assert.Greater(t, det.extractCalls, 0, "first call must read sprite")
}

func TestExtractEpisodeHashesCached_EmptyFingerprint_SkipsCache(t *testing.T) {
	t.Parallel()
	cache := libraryfakes.NewFakeIntroFingerprintCacheRepository()
	det := &hashesDetector{tileHashes: []uint64{1, 2, 3}}
	svc, manifest := newServiceWithCache(t, det, cache)
	cutoff := manifestTotalFrames(manifest)

	_, _, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "", manifest, cutoff)
	require.NoError(t, err)
	assert.Equal(t, 0, cache.Count(), "empty fingerprint must not write to cache")
}

func TestExtractEpisodeHashesCached_HitOnSecondCall(t *testing.T) {
	t.Parallel()
	cache := libraryfakes.NewFakeIntroFingerprintCacheRepository()
	det := &hashesDetector{tileHashes: []uint64{10, 20, 30}}
	svc, manifest := newServiceWithCache(t, det, cache)
	cutoff := manifestTotalFrames(manifest)

	// First call: cache miss → extract + upsert.
	hashes1, ts1, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "fp-A", manifest, cutoff)
	require.NoError(t, err)
	require.Len(t, hashes1, 3)
	require.Len(t, ts1, 3)
	assert.Equal(t, 1, cache.Count(), "first call must populate cache")
	callsAfterFirst := det.extractCalls

	// Second call: must hit cache; no additional sprite reads.
	hashes2, ts2, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "fp-A", manifest, cutoff)
	require.NoError(t, err, "second call must hit cache")
	assert.Equal(t, callsAfterFirst, det.extractCalls, "cache hit must not re-read sprites")
	assert.Equal(t, hashes1, hashes2)
	assert.Equal(t, ts1, ts2)
}

func TestExtractEpisodeHashesCached_FingerprintMismatch_Recomputes(t *testing.T) {
	t.Parallel()
	cache := libraryfakes.NewFakeIntroFingerprintCacheRepository()
	require.NoError(t, cache.Upsert(context.Background(), &domainlibrary.IntroFingerprintCacheEntry{
		MediaItemID:     "media-1",
		FileFingerprint: "fp-OLD",
		PhashTiles:      []uint64{99, 99, 99},
		PhashTimestamps: []int64{0, 1000, 2000},
		AlgoVersion:     currentPHashAlgoVersion,
	}))
	det := &hashesDetector{tileHashes: []uint64{1, 2, 3}}
	svc, manifest := newServiceWithCache(t, det, cache)
	cutoff := manifestTotalFrames(manifest)

	hashes, _, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "fp-NEW", manifest, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []uint64{1, 2, 3}, hashes, "stale fingerprint must trigger recompute")

	updated, err := cache.Get(context.Background(), "media-1")
	require.NoError(t, err)
	assert.Equal(t, "fp-NEW", updated.FileFingerprint, "cache entry must be refreshed with new fingerprint")
}

func TestExtractEpisodeHashesCached_AlgoMismatch_Recomputes(t *testing.T) {
	t.Parallel()
	cache := libraryfakes.NewFakeIntroFingerprintCacheRepository()
	require.NoError(t, cache.Upsert(context.Background(), &domainlibrary.IntroFingerprintCacheEntry{
		MediaItemID:     "media-1",
		FileFingerprint: "fp-A",
		PhashTiles:      []uint64{99},
		PhashTimestamps: []int64{0},
		AlgoVersion:     currentPHashAlgoVersion - 1,
	}))
	det := &hashesDetector{tileHashes: []uint64{1, 2, 3}}
	svc, manifest := newServiceWithCache(t, det, cache)
	cutoff := manifestTotalFrames(manifest)

	hashes, _, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "fp-A", manifest, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []uint64{1, 2, 3}, hashes, "stale algo_version must trigger recompute")

	updated, err := cache.Get(context.Background(), "media-1")
	require.NoError(t, err)
	assert.Equal(t, currentPHashAlgoVersion, updated.AlgoVersion)
}

func TestExtractEpisodeHashesCached_TruncateOnHit(t *testing.T) {
	t.Parallel()
	cache := libraryfakes.NewFakeIntroFingerprintCacheRepository()
	det := &hashesDetector{tileHashes: []uint64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150}}
	svc, manifest := newServiceWithCache(t, det, cache)
	cutoff := manifestTotalFrames(manifest)

	// Populate cache (first call extracts all 15 frames).
	_, _, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "fp-A", manifest, cutoff)
	require.NoError(t, err)

	// Second call with smaller cutoff: cache hit + truncate to first 5.
	hashes, ts, err := svc.extractEpisodeHashesCached(context.Background(), "media-1", "fp-A", manifest, 5)
	require.NoError(t, err)
	require.Len(t, hashes, 5)
	require.Len(t, ts, 5)
	assert.Equal(t, []uint64{10, 20, 30, 40, 50}, hashes)
}
