package library

import (
	"context"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrequencyMap_AllPresent verifies that hashes present in all episodes are returned.
func TestFrequencyMap_AllPresent(t *testing.T) {
	t.Parallel()
	var hashA, hashB, hashC uint64 = 0xAAAA, 0xBBBB, 0xCCCC

	episodeHashes := [][]uint64{
		{hashA, hashB},
		{hashA, hashB},
		{hashA, hashC},
	}

	result := FrequencyMap(episodeHashes, 0.70)
	require.NotNil(t, result)
	assert.True(t, result[hashA], "hashA in 3/3 episodes must be in result")
	assert.False(t, result[hashB], "hashB in 2/3 (67%%) must NOT be in result at threshold 70%%")
	assert.False(t, result[hashC], "hashC in 1/3 must NOT be in result")
}

// TestFrequencyMap_ThresholdBoundary verifies exact threshold behaviour.
func TestFrequencyMap_ThresholdBoundary(t *testing.T) {
	t.Parallel()
	var hA, hB uint64 = 0x1111, 0x2222

	episodeHashes := [][]uint64{
		{hA, hB},
		{hA},
	}

	// At threshold 0.5: hA (2/2=100%) ✓, hB (1/2=50%) on the boundary.
	result := FrequencyMap(episodeHashes, 0.50)
	assert.True(t, result[hA])
	assert.True(t, result[hB], "exactly at threshold (50%%) must be included")

	// At threshold 0.70: hB (50%%) must be excluded.
	result70 := FrequencyMap(episodeHashes, 0.70)
	assert.True(t, result70[hA])
	assert.False(t, result70[hB])
}

// TestFrequencyMap_Empty handles an empty input gracefully.
func TestFrequencyMap_Empty(t *testing.T) {
	t.Parallel()
	result := FrequencyMap(nil, 0.70)
	assert.Nil(t, result)

	result2 := FrequencyMap([][]uint64{}, 0.70)
	assert.Nil(t, result2)
}

// TestFrequencyMap_SingleEpisode: with one episode, any hash reaches 100%%.
func TestFrequencyMap_SingleEpisode(t *testing.T) {
	t.Parallel()
	var h uint64 = 0xDEAD
	result := FrequencyMap([][]uint64{{h, h + 1}}, 0.70)
	assert.True(t, result[h])
	assert.True(t, result[h+1])
}

// TestFrequencyMap_DuplicatesCountedOnce verifies that duplicates within the same
// episode are not double-counted.
func TestFrequencyMap_DuplicatesCountedOnce(t *testing.T) {
	t.Parallel()
	// hash 0x1 appears 3x in ep1, 0x in ep2 → should count as 1/2 = 50%, not 3/2.
	var h uint64 = 0x0001
	episodeHashes := [][]uint64{
		{h, h, h},
		{0xFFFF},
	}
	result := FrequencyMap(episodeHashes, 0.70)
	assert.False(t, result[h], "duplicate hashes within same episode must only count once")
}

// TestLongestContiguousWindow_BasicMatch ensures a matching run is found.
func TestLongestContiguousWindow_BasicMatch(t *testing.T) {
	t.Parallel()
	introHashes := map[uint64]bool{0xAA: true, 0xBB: true, 0xCC: true}
	// Target: [0xDD, 0xAA, 0xBB, 0xCC, 0xEE]
	// Longest run of intro hashes: indices 1,2,3 → 3 frames.
	target := []uint64{0xDD, 0xAA, 0xBB, 0xCC, 0xEE}
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}

	startMs, endMs, found := longestContiguousWindow(manifest, introHashes, target)
	require.True(t, found)
	assert.Equal(t, int64(1*10*1000), startMs)
	assert.Equal(t, int64(4*10*1000), endMs)
}

// TestLongestContiguousWindow_NoMatch returns not-found when no hash matches.
func TestLongestContiguousWindow_NoMatch(t *testing.T) {
	t.Parallel()
	introHashes := map[uint64]bool{0xAA: true}
	target := []uint64{0x11, 0x22, 0x33}
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}

	_, _, found := longestContiguousWindow(manifest, introHashes, target)
	assert.False(t, found)
}

// TestLongestContiguousWindow_EmptyTarget returns not-found on empty hash slice.
func TestLongestContiguousWindow_EmptyTarget(t *testing.T) {
	t.Parallel()
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}
	_, _, found := longestContiguousWindow(manifest, map[uint64]bool{0x1: true}, []uint64{})
	assert.False(t, found)
}

// TestLongestContiguousWindow_AllMatch verifies full-sequence match.
func TestLongestContiguousWindow_AllMatch(t *testing.T) {
	t.Parallel()
	var h1, h2 uint64 = 0x01, 0x02
	introHashes := map[uint64]bool{h1: true, h2: true}
	target := []uint64{h1, h2, h1}
	manifest := &domainlibrary.TrickplayManifest{Interval: 5}

	startMs, endMs, found := longestContiguousWindow(manifest, introHashes, target)
	require.True(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(3*5*1000), endMs)
}

// --- topTwoContiguousWindows tests ---

// TestTopTwoContiguousWindows_Empty returns nil for an empty target.
func TestTopTwoContiguousWindows_Empty(t *testing.T) {
	t.Parallel()
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}
	result := topTwoContiguousWindows(manifest, map[uint64]bool{0x1: true}, nil)
	assert.Nil(t, result)

	result2 := topTwoContiguousWindows(manifest, map[uint64]bool{0x1: true}, []uint64{})
	assert.Nil(t, result2)
}

// TestTopTwoContiguousWindows_ZeroInterval returns nil for zero interval.
func TestTopTwoContiguousWindows_ZeroInterval(t *testing.T) {
	t.Parallel()
	manifest := &domainlibrary.TrickplayManifest{Interval: 0}
	result := topTwoContiguousWindows(manifest, map[uint64]bool{0x1: true}, []uint64{0x1, 0x1})
	assert.Nil(t, result)
}

// TestTopTwoContiguousWindows_SingleRun returns a single window.
func TestTopTwoContiguousWindows_SingleRun(t *testing.T) {
	t.Parallel()
	// indices 1,2,3 match → 1 run of length 3
	introHashes := map[uint64]bool{0xAA: true, 0xBB: true, 0xCC: true}
	target := []uint64{0x00, 0xAA, 0xBB, 0xCC, 0x00}
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}

	result := topTwoContiguousWindows(manifest, introHashes, target)
	require.Len(t, result, 1)
	assert.Equal(t, int64(1*10*1000), result[0].startMs)
	assert.Equal(t, int64(4*10*1000), result[0].endMs)
}

// TestTopTwoContiguousWindows_TwoEqualRuns returns both, sorted chronologically.
func TestTopTwoContiguousWindows_TwoEqualRuns(t *testing.T) {
	t.Parallel()
	// run1: indices 0,1 (AA,BB), run2: indices 3,4 (AA,BB)
	introHashes := map[uint64]bool{0xAA: true, 0xBB: true}
	target := []uint64{0xAA, 0xBB, 0x00, 0xAA, 0xBB}
	manifest := &domainlibrary.TrickplayManifest{Interval: 5}

	result := topTwoContiguousWindows(manifest, introHashes, target)
	require.Len(t, result, 2)
	// Chronological: earlier run first.
	assert.Equal(t, int64(0), result[0].startMs)
	assert.Equal(t, int64(2*5*1000), result[0].endMs)
	assert.Equal(t, int64(3*5*1000), result[1].startMs)
	assert.Equal(t, int64(5*5*1000), result[1].endMs)
}

// TestTopTwoContiguousWindows_ThreeRunsKeepsTopTwo verifies only the two longest runs are kept.
func TestTopTwoContiguousWindows_ThreeRunsKeepsTopTwo(t *testing.T) {
	t.Parallel()
	// run1(len=3): 0,1,2  run2(len=1): 4  run3(len=2): 6,7
	// Top 2 by length: run1(3) and run3(2). Chronological: run1 then run3.
	introHashes := map[uint64]bool{0xAA: true, 0xBB: true, 0xCC: true}
	target := []uint64{0xAA, 0xBB, 0xCC, 0x00, 0xAA, 0x00, 0xAA, 0xBB}
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}

	result := topTwoContiguousWindows(manifest, introHashes, target)
	require.Len(t, result, 2)
	// run1: start=0, end=3
	assert.Equal(t, int64(0), result[0].startMs)
	assert.Equal(t, int64(3*10*1000), result[0].endMs)
	// run3: start=6, end=8
	assert.Equal(t, int64(6*10*1000), result[1].startMs)
	assert.Equal(t, int64(8*10*1000), result[1].endMs)
}

// TestTopTwoContiguousWindows_RunAtEnd verifies a run ending at the last frame.
func TestTopTwoContiguousWindows_RunAtEnd(t *testing.T) {
	t.Parallel()
	introHashes := map[uint64]bool{0xAA: true}
	target := []uint64{0x00, 0xAA, 0xAA}
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}

	result := topTwoContiguousWindows(manifest, introHashes, target)
	require.Len(t, result, 1)
	assert.Equal(t, int64(1*10*1000), result[0].startMs)
	assert.Equal(t, int64(3*10*1000), result[0].endMs)
}

// --- classifySegments tests ---

// TestClassifySegments_NoWindows returns empty slice.
func TestClassifySegments_NoWindows(t *testing.T) {
	t.Parallel()
	segs := classifySegments(nil)
	assert.Empty(t, segs)

	segs2 := classifySegments([]matchWindow{})
	assert.Empty(t, segs2)
}

// TestClassifySegments_OneWindow produces a single intro segment.
func TestClassifySegments_OneWindow(t *testing.T) {
	t.Parallel()
	windows := []matchWindow{{startMs: 5000, endMs: 90000}}
	segs := classifySegments(windows)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
	assert.Equal(t, int64(5000), segs[0].StartMs)
	assert.Equal(t, int64(90000), segs[0].EndMs)
}

// TestClassifySegments_TwoWindowsEarlierShort produces recap + intro when earlier < 60s.
func TestClassifySegments_TwoWindowsEarlierShort(t *testing.T) {
	t.Parallel()
	// earlier window: 0–30s (30s < 60s → recap)
	// later window: 60s–150s (intro)
	windows := []matchWindow{
		{startMs: 0, endMs: 30_000},
		{startMs: 60_000, endMs: 150_000},
	}
	segs := classifySegments(windows)
	require.Len(t, segs, 2)
	assert.Equal(t, "recap", segs[0].Type)
	assert.Equal(t, int64(0), segs[0].StartMs)
	assert.Equal(t, int64(30_000), segs[0].EndMs)
	assert.Equal(t, "intro", segs[1].Type)
	assert.Equal(t, int64(60_000), segs[1].StartMs)
	assert.Equal(t, int64(150_000), segs[1].EndMs)
}

// TestClassifySegments_TwoWindowsEarlierLong returns only the largest intro when earlier ≥ 60s.
func TestClassifySegments_TwoWindowsEarlierLong(t *testing.T) {
	t.Parallel()
	// earlier window: 0–90s (90s ≥ 60s), later window: 120s–180s (60s)
	// earlier is bigger → only earlier returned as intro
	windows := []matchWindow{
		{startMs: 0, endMs: 90_000},
		{startMs: 120_000, endMs: 180_000},
	}
	segs := classifySegments(windows)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
	assert.Equal(t, int64(0), segs[0].StartMs)
	assert.Equal(t, int64(90_000), segs[0].EndMs)
}

// TestClassifySegments_TwoWindowsEarlierLongLaterBigger: earlier ≥60s but later is bigger → later is intro.
func TestClassifySegments_TwoWindowsEarlierLongLaterBigger(t *testing.T) {
	t.Parallel()
	// earlier: 0–70s (70s), later: 100s–200s (100s → bigger)
	windows := []matchWindow{
		{startMs: 0, endMs: 70_000},
		{startMs: 100_000, endMs: 200_000},
	}
	segs := classifySegments(windows)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
	assert.Equal(t, int64(100_000), segs[0].StartMs)
	assert.Equal(t, int64(200_000), segs[0].EndMs)
}

// --- end credits service integration tests ---

// fakeDetector implements IntroDetector for tests.
// ExtractTileHashes, PrecisionPass, AudioFallback return zero/not-found;
// DetectEndCredits and DetectStudioLogo return the configured responses.
type fakeDetector struct {
	creditsStartMs int64
	creditsEndMs   int64
	creditsFound   bool
	creditsErr     error

	logoStartMs int64
	logoEndMs   int64
	logoFound   bool

	// tileHashes is returned by ExtractTileHashes when non-nil; defaults to nil (no hashes).
	tileHashes []uint64
}

func (f *fakeDetector) ExtractTileHashes(_ string, _, _, _, _ int) ([]uint64, error) {
	return f.tileHashes, nil
}
func (f *fakeDetector) PrecisionPass(_ context.Context, _ string, startSec, endSec, _ float64) (int64, int64, error) {
	return int64(startSec * 1000), int64(endSec * 1000), nil
}
func (f *fakeDetector) AudioFallback(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (f *fakeDetector) DetectEndCredits(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	return f.creditsStartMs, f.creditsEndMs, f.creditsFound, f.creditsErr
}
func (f *fakeDetector) DetectStudioLogo(_ []uint64, _ int) (int64, int64, bool) {
	return f.logoStartMs, f.logoEndMs, f.logoFound
}

// fakeSegmentRepo records all Upsert calls and stores segments by (mediaItemID, type).
// GetByMediaItemID returns segments that were previously upserted.
type fakeSegmentRepo struct {
	upserted []*domainlibrary.MediaSegment
	// seeded allows tests to pre-populate segments for GetByMediaItemID.
	seeded []*domainlibrary.MediaSegment
	// deleted records (mediaItemID, type) pairs passed to DeleteByMediaItemIDAndType.
	deleted []string
}

func (r *fakeSegmentRepo) Upsert(_ context.Context, seg *domainlibrary.MediaSegment) error {
	r.upserted = append(r.upserted, seg)
	return nil
}
func (r *fakeSegmentRepo) GetByMediaItemID(_ context.Context, id shared.MediaItemID) ([]*domainlibrary.MediaSegment, error) {
	var out []*domainlibrary.MediaSegment
	for _, s := range r.seeded {
		if s.MediaItemID == id {
			out = append(out, s)
		}
	}
	// Also include previously upserted segments.
	for _, s := range r.upserted {
		if s.MediaItemID == id {
			out = append(out, s)
		}
	}
	return out, nil
}
func (r *fakeSegmentRepo) DeleteByMediaItemID(_ context.Context, _ shared.MediaItemID) error {
	return nil
}
func (r *fakeSegmentRepo) DeleteByMediaItemIDAndType(_ context.Context, id shared.MediaItemID, segType string) error {
	r.deleted = append(r.deleted, string(id)+"/"+segType)
	return nil
}
func (r *fakeSegmentRepo) ListSeasonsMissingSegments(_ context.Context) ([]string, error) {
	return nil, nil
}

// fakeMediaItemRepo returns the configured item for GetByID calls and optionally
// a list of siblings for List calls.
type fakeMediaItemRepo struct {
	item     *domainlibrary.MediaItem
	siblings []*domainlibrary.MediaItem
}

func (r *fakeMediaItemRepo) GetByID(_ context.Context, _ shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	return r.item, nil
}
func (r *fakeMediaItemRepo) Create(_ context.Context, _ *domainlibrary.MediaItem) error { return nil }
func (r *fakeMediaItemRepo) List(_ context.Context, _ domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	return r.siblings, nil
}
func (r *fakeMediaItemRepo) Update(_ context.Context, _ *domainlibrary.MediaItem) error { return nil }
func (r *fakeMediaItemRepo) Delete(_ context.Context, _ shared.MediaItemID) error       { return nil }
func (r *fakeMediaItemRepo) GetByFilePath(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}
func (r *fakeMediaItemRepo) GetByFileFingerprint(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}
func (r *fakeMediaItemRepo) GetByLibraryTypeTitle(_ context.Context, _ shared.LibraryID, _, _, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}
func (r *fakeMediaItemRepo) Count(_ context.Context, _ domainlibrary.ListMediaItemsFilter) (int, error) {
	return 0, nil
}
func (r *fakeMediaItemRepo) SeriesAggregate(_ context.Context, _ shared.MediaItemID) (domainlibrary.SeriesAggregate, error) {
	return domainlibrary.SeriesAggregate{}, nil
}

// fakeTrickplayRepo returns ErrNotFound for any lookup so the pHash pass is skipped.
type fakeTrickplayRepo struct{}

// fakeTrickplayRepoWithManifest returns the provided manifest for any lookup.
// Used to exercise detectStudioLogo, which queries the manifest non-blocking.
type fakeTrickplayRepoWithManifest struct {
	manifest *domainlibrary.TrickplayManifest
}

func (r *fakeTrickplayRepo) GetByMediaItemID(_ context.Context, _ shared.MediaItemID) (*domainlibrary.TrickplayManifest, error) {
	return nil, shared.ErrNotFound
}
func (r *fakeTrickplayRepo) ListMissingMediaItemIDs(_ context.Context) ([]string, error) {
	return nil, nil
}
func (r *fakeTrickplayRepo) Create(_ context.Context, _ *domainlibrary.TrickplayManifest) error {
	return nil
}
func (r *fakeTrickplayRepo) Delete(_ context.Context, _ shared.MediaItemID) error { return nil }

func (r *fakeTrickplayRepoWithManifest) GetByMediaItemID(_ context.Context, _ shared.MediaItemID) (*domainlibrary.TrickplayManifest, error) {
	return r.manifest, nil
}
func (r *fakeTrickplayRepoWithManifest) ListMissingMediaItemIDs(_ context.Context) ([]string, error) {
	return nil, nil
}
func (r *fakeTrickplayRepoWithManifest) Create(_ context.Context, _ *domainlibrary.TrickplayManifest) error {
	return nil
}
func (r *fakeTrickplayRepoWithManifest) Delete(_ context.Context, _ shared.MediaItemID) error {
	return nil
}

// fakeConfigRepo disables trickplay so the pHash pass is skipped immediately,
// preventing the 24h waitForManifest loop from blocking tests.
type fakeConfigRepo struct{}

func (r *fakeConfigRepo) Get(_ context.Context, _ string) (*shared.ServerConfigEntry, error) {
	return &shared.ServerConfigEntry{Key: "trickplay.enabled", Value: "false"}, nil
}
func (r *fakeConfigRepo) Set(_ context.Context, _, _ string) error   { return nil }
func (r *fakeConfigRepo) Delete(_ context.Context, _ string) error   { return nil }
func (r *fakeConfigRepo) List(_ context.Context) ([]*shared.ServerConfigEntry, error) {
	return nil, nil
}

// newTestService assembles a minimal IntroDetectionService for unit tests.
func newTestService(det IntroDetector, segRepo domainlibrary.MediaSegmentRepository, item *domainlibrary.MediaItem) *IntroDetectionService {
	return NewIntroDetectionService(
		segRepo,
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		det,
		"/tmp/trickplay",
	)
}

// newTestServiceWithTrickplay assembles an IntroDetectionService with a manifest-returning trickplay repo.
func newTestServiceWithTrickplay(det IntroDetector, segRepo domainlibrary.MediaSegmentRepository, item *domainlibrary.MediaItem, manifest *domainlibrary.TrickplayManifest) *IntroDetectionService {
	return NewIntroDetectionService(
		segRepo,
		&fakeTrickplayRepoWithManifest{manifest: manifest},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		det,
		"/tmp/trickplay",
	)
}

// newTestServiceFull assembles an IntroDetectionService with a custom media item repo and trickplay repo.
func newTestServiceFull(det IntroDetector, segRepo domainlibrary.MediaSegmentRepository, itemRepo *fakeMediaItemRepo, trickRepo domainlibrary.TrickplayRepository) *IntroDetectionService {
	return NewIntroDetectionService(
		segRepo,
		trickRepo,
		itemRepo,
		&fakeConfigRepo{},
		det,
		"/tmp/trickplay",
	)
}

// TestDetectIntro_AppendsEndCreditsWhenDetected verifies that when DetectEndCredits
// returns found=true the resulting segment is upserted with type "end_credits".
func TestDetectIntro_AppendsEndCreditsWhenDetected(t *testing.T) {
	t.Parallel()

	dur, _ := shared.NewDurationMs(3_600_000) // 60 min
	item := &domainlibrary.MediaItem{
		ID:        shared.MediaItemID("item-1"),
		LibraryID: shared.LibraryID("lib-1"),
		Type:      domainlibrary.MovieItem,
		Title:     "Test Movie",
		File: &domainlibrary.MediaFile{
			Path:     "/movies/test.mkv",
			Duration: dur,
		},
	}

	det := &fakeDetector{
		creditsStartMs: 3_480_000, // 58 min
		creditsEndMs:   3_600_000, // 60 min
		creditsFound:   true,
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(det, segRepo, item)

	err := svc.DetectIntro(context.Background(), "item-1")
	require.NoError(t, err)

	require.Len(t, segRepo.upserted, 1)
	assert.Equal(t, "end_credits", segRepo.upserted[0].Type)
	assert.Equal(t, int64(3_480_000), segRepo.upserted[0].StartMs)
	assert.Equal(t, int64(3_600_000), segRepo.upserted[0].EndMs)
	assert.InDelta(t, 0.6, segRepo.upserted[0].Confidence, 0.001)
	assert.Equal(t, "blackdetect", segRepo.upserted[0].Source)
}

// TestDetectIntro_SkipsEndCreditsWhenNotFound verifies that when DetectEndCredits
// returns found=false no end_credits segment is upserted.
func TestDetectIntro_SkipsEndCreditsWhenNotFound(t *testing.T) {
	t.Parallel()

	dur, _ := shared.NewDurationMs(3_600_000)
	item := &domainlibrary.MediaItem{
		ID:        shared.MediaItemID("item-2"),
		LibraryID: shared.LibraryID("lib-1"),
		Type:      domainlibrary.MovieItem,
		Title:     "Test Movie 2",
		File: &domainlibrary.MediaFile{
			Path:     "/movies/test2.mkv",
			Duration: dur,
		},
	}

	det := &fakeDetector{creditsFound: false}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(det, segRepo, item)

	err := svc.DetectIntro(context.Background(), "item-2")
	require.NoError(t, err)

	for _, seg := range segRepo.upserted {
		assert.NotEqual(t, "end_credits", seg.Type, "no end_credits segment should be upserted when not found")
	}
}

// TestDetectIntro_AppendsStudioLogoWhenDetected verifies that when DetectStudioLogo
// returns found=true the resulting segment is upserted with type "studio_logo".
func TestDetectIntro_AppendsStudioLogoWhenDetected(t *testing.T) {
	t.Parallel()

	dur, _ := shared.NewDurationMs(3_600_000) // 60 min
	item := &domainlibrary.MediaItem{
		ID:        shared.MediaItemID("item-logo-1"),
		LibraryID: shared.LibraryID("lib-1"),
		Type:      domainlibrary.MovieItem,
		Title:     "Test Movie Logo",
		File: &domainlibrary.MediaFile{
			Path:     "/movies/logo_test.mkv",
			Duration: dur,
		},
	}

	// Manifest with Interval=15 so detectStudioLogo computes maxFrames=6.
	manifest := &domainlibrary.TrickplayManifest{
		MediaItemID: item.ID,
		Interval:    15,
		TileWidth:   5,
		TileHeight:  3,
		Width:       160,
		Height:      90,
		ImageCount:  1,
	}

	det := &fakeDetector{
		// tileHashes causes extractEpisodeHashes to return a non-empty slice so
		// the logo detection path is reached.
		tileHashes:  []uint64{0x1111, 0x2222, 0x3333},
		logoStartMs: 0,
		logoEndMs:   5_000,
		logoFound:   true,
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestServiceWithTrickplay(det, segRepo, item, manifest)

	err := svc.DetectIntro(context.Background(), "item-logo-1")
	require.NoError(t, err)

	var logoSeg *domainlibrary.MediaSegment
	for _, seg := range segRepo.upserted {
		if seg.Type == "studio_logo" {
			logoSeg = seg
			break
		}
	}
	require.NotNil(t, logoSeg, "studio_logo segment must be upserted when DetectStudioLogo returns found=true")
	assert.Equal(t, int64(0), logoSeg.StartMs)
	assert.Equal(t, int64(5_000), logoSeg.EndMs)
	assert.InDelta(t, 0.7, logoSeg.Confidence, 0.001)
	assert.Equal(t, "logo_match", logoSeg.Source)
}

// --- SetManualSegment tests ---

func TestSetManualSegment_RejectsBadType(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3_600_000)
	item := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("item-manual-1"),
		File: &domainlibrary.MediaFile{
			Path:     "/ep.mkv",
			Duration: dur,
		},
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(&fakeDetector{}, segRepo, item)

	err := svc.SetManualSegment(context.Background(), "item-manual-1", "unknown_type", 0, 5000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid segment type")
	assert.Empty(t, segRepo.upserted)
}

func TestSetManualSegment_RejectsBadRange(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3_600_000)
	item := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("item-manual-2"),
		File: &domainlibrary.MediaFile{
			Path:     "/ep.mkv",
			Duration: dur,
		},
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(&fakeDetector{}, segRepo, item)

	// endMs == startMs
	err := svc.SetManualSegment(context.Background(), "item-manual-2", "intro", 5000, 5000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "end_ms must be > start_ms")

	// endMs < startMs
	err = svc.SetManualSegment(context.Background(), "item-manual-2", "intro", 5000, 4000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "end_ms must be > start_ms")
}

func TestSetManualSegment_UpsertsWithSourceManual(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3_600_000)
	item := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("item-manual-3"),
		File: &domainlibrary.MediaFile{
			Path:     "/ep.mkv",
			Duration: dur,
		},
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(&fakeDetector{}, segRepo, item)

	err := svc.SetManualSegment(context.Background(), "item-manual-3", "intro", 10_000, 60_000)
	require.NoError(t, err)
	require.Len(t, segRepo.upserted, 1)
	seg := segRepo.upserted[0]
	assert.Equal(t, "intro", seg.Type)
	assert.Equal(t, int64(10_000), seg.StartMs)
	assert.Equal(t, int64(60_000), seg.EndMs)
	assert.Equal(t, "manual", seg.Source)
	assert.InDelta(t, 1.0, seg.Confidence, 0.001)
	assert.NotEmpty(t, seg.ID)
}

func TestSetManualSegment_RejectsEndExceedingDuration(t *testing.T) {
	t.Parallel()
	// Duration = 60s = 60_000 ms
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("item-manual-4"),
		File: &domainlibrary.MediaFile{
			Path:     "/ep.mkv",
			Duration: dur,
		},
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(&fakeDetector{}, segRepo, item)

	err := svc.SetManualSegment(context.Background(), "item-manual-4", "intro", 0, 70_000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds media duration")
}

// --- Manual lock tests ---

func TestDetectIntro_RespectsManualLock(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3_600_000)
	mediaID := shared.MediaItemID("item-lock-1")
	item := &domainlibrary.MediaItem{
		ID:   mediaID,
		Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{
			Path:     "/ep.mkv",
			Duration: dur,
		},
	}
	// Pre-seed a manual lock for the "end_credits" type.
	existing := &domainlibrary.MediaSegment{
		ID:          "existing-1",
		MediaItemID: mediaID,
		Type:        "end_credits",
		StartMs:     3_400_000,
		EndMs:       3_600_000,
		Source:      "manual",
		Confidence:  1.0,
	}
	segRepo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{existing}}
	det := &fakeDetector{
		creditsStartMs: 3_480_000,
		creditsEndMs:   3_600_000,
		creditsFound:   true,
	}
	svc := newTestService(det, segRepo, item)

	err := svc.DetectIntro(context.Background(), string(mediaID))
	require.NoError(t, err)

	// The auto-detected end_credits must NOT overwrite the manual one.
	for _, seg := range segRepo.upserted {
		assert.NotEqual(t, "end_credits", seg.Type, "auto detection must not upsert when manual lock exists")
	}
}

func TestDetectIntro_RespectsManualPropagatedLock(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3_600_000)
	mediaID := shared.MediaItemID("item-lock-2")
	item := &domainlibrary.MediaItem{
		ID:   mediaID,
		Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{
			Path:     "/ep.mkv",
			Duration: dur,
		},
	}
	existing := &domainlibrary.MediaSegment{
		ID:          "existing-2",
		MediaItemID: mediaID,
		Type:        "end_credits",
		StartMs:     3_400_000,
		EndMs:       3_600_000,
		Source:      "manual_propagated",
		Confidence:  0.85,
	}
	segRepo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{existing}}
	det := &fakeDetector{
		creditsStartMs: 3_480_000,
		creditsEndMs:   3_600_000,
		creditsFound:   true,
	}
	svc := newTestService(det, segRepo, item)

	err := svc.DetectIntro(context.Background(), string(mediaID))
	require.NoError(t, err)

	for _, seg := range segRepo.upserted {
		assert.NotEqual(t, "end_credits", seg.Type, "auto detection must not upsert when manual_propagated lock exists")
	}
}

// --- DeleteSegment tests ---

func TestDeleteSegment_CallsRepoWithType(t *testing.T) {
	t.Parallel()
	item := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("item-del-1"),
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(&fakeDetector{}, segRepo, item)

	err := svc.DeleteSegment(context.Background(), "item-del-1", "intro")
	require.NoError(t, err)
	require.Len(t, segRepo.deleted, 1)
	assert.Equal(t, "item-del-1/intro", segRepo.deleted[0])
}

// --- PropagateSegment tests ---

func TestPropagateSegment_FailsForFilms(t *testing.T) {
	t.Parallel()
	item := &domainlibrary.MediaItem{
		ID:   shared.MediaItemID("movie-1"),
		Type: domainlibrary.MovieItem,
	}
	segRepo := &fakeSegmentRepo{}
	svc := newTestService(&fakeDetector{}, segRepo, item)

	_, err := svc.PropagateSegment(context.Background(), "movie-1", "intro")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "propagate only works on episodes")
}

func TestPropagateSegment_FailsWithoutReferenceSegment(t *testing.T) {
	t.Parallel()
	parentID := "season-1"
	item := &domainlibrary.MediaItem{
		ID:       shared.MediaItemID("ep-ref-1"),
		Type:     domainlibrary.Episode,
		ParentID: parentID,
	}
	segRepo := &fakeSegmentRepo{} // no segments seeded
	svc := newTestService(&fakeDetector{}, segRepo, item)

	_, err := svc.PropagateSegment(context.Background(), "ep-ref-1", "intro")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no intro segment on reference episode")
}

// --- pickWindowMatchingDuration tests ---

func TestPickWindowMatchingDuration_PicksClosestToDuration(t *testing.T) {
	t.Parallel()
	// refDur = 15_000 ms
	// windows: {0,5000}=5s (too short: 5*2=10 < 15), {0,16000}=16s (ok), {0,30000}=30s (30>15*2=30 not, exactly 30=15*2, borderline)
	windows := []matchWindow{
		{startMs: 0, endMs: 5_000},
		{startMs: 0, endMs: 16_000},
		{startMs: 0, endMs: 30_000},
	}
	result := pickWindowMatchingDuration(windows, 15_000)
	// {0,5000}: dur=5000, 5000*2=10000 < 15000 → rejected
	// {0,16000}: dur=16000, 16000*2=32000 >= 15000 and 16000 <= 15000*2=30000 → ok, delta=1000
	// {0,30000}: dur=30000, 30000*2=60000 >= 15000 and 30000 <= 30000 → ok, delta=15000
	// Best: 16000 (delta=1000 < 15000)
	require.NotNil(t, result)
	assert.Equal(t, int64(16_000), result.endMs)
}

func TestPickWindowMatchingDuration_TooShortReturnsNil(t *testing.T) {
	t.Parallel()
	windows := []matchWindow{{startMs: 0, endMs: 2_000}}
	result := pickWindowMatchingDuration(windows, 15_000)
	// 2000*2=4000 < 15000 → rejected
	assert.Nil(t, result)
}

func TestPickWindowMatchingDuration_TooLongReturnsNil(t *testing.T) {
	t.Parallel()
	windows := []matchWindow{{startMs: 0, endMs: 90_000}}
	result := pickWindowMatchingDuration(windows, 15_000)
	// 90000 > 15000*2=30000 → rejected
	assert.Nil(t, result)
}
