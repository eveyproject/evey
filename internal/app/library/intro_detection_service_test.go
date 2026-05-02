package library

import (
	"context"
	"fmt"
	"os"
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

// --- allCandidatesGapTolerant tests ---

// msSeq builds a slice of timestamps with constant spacing — convenience for
// tests that don't care about jitter and want N frames every `step` ms.
func msSeq(n int, step int64) []int64 {
	out := make([]int64, n)
	for i := range n {
		out[i] = int64(i) * step
	}
	return out
}

func TestAllCandidatesGapTolerant_Empty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, allCandidatesGapTolerant(map[uint64]bool{0x1: true}, nil, nil, 0))
	assert.Nil(t, allCandidatesGapTolerant(map[uint64]bool{0x1: true}, []uint64{}, []int64{}, 0))
}

func TestAllCandidatesGapTolerant_PureContiguousRun(t *testing.T) {
	t.Parallel()
	intro := map[uint64]bool{0xAA: true, 0xBB: true, 0xCC: true}
	target := []uint64{0x00, 0xAA, 0xBB, 0xCC, 0x00}
	ms := msSeq(5, 10_000) // 0, 10s, 20s, 30s, 40s
	cands := allCandidatesGapTolerant(intro, target, ms, 0)
	require.Len(t, cands, 1)
	// Run is frames 1..3 (inclusive). startMs=ms[1], endMs=ms[4] (next-frame ms).
	assert.Equal(t, int64(10_000), cands[0].startMs)
	assert.Equal(t, int64(40_000), cands[0].endMs)
	assert.Equal(t, 3, cands[0].matchCount)
}

func TestAllCandidatesGapTolerant_MergesGap1(t *testing.T) {
	t.Parallel()
	intro := map[uint64]bool{0xAA: true, 0xBB: true}
	target := []uint64{0xAA, 0xAA, 0x00, 0xBB, 0xBB}
	ms := msSeq(5, 10_000)
	cands := allCandidatesGapTolerant(intro, target, ms, 0)
	require.Len(t, cands, 1)
	assert.Equal(t, int64(0), cands[0].startMs)
	// Last match at frame 4, end_ms uses last matched (no frame 5 in slice).
	assert.Equal(t, int64(40_000), cands[0].endMs)
	assert.Equal(t, 4, cands[0].matchCount)
}

func TestAllCandidatesGapTolerant_DoesNotMergeGap3(t *testing.T) {
	t.Parallel()
	intro := map[uint64]bool{0xAA: true, 0xBB: true}
	target := []uint64{0xAA, 0xAA, 0x00, 0x00, 0x00, 0xBB, 0xBB}
	ms := msSeq(7, 10_000)
	cands := allCandidatesGapTolerant(intro, target, ms, 0)
	require.Len(t, cands, 2)
	assert.Equal(t, int64(0), cands[0].startMs)
	assert.Equal(t, int64(20_000), cands[0].endMs)
	assert.Equal(t, int64(50_000), cands[1].startMs)
	assert.Equal(t, int64(60_000), cands[1].endMs)
}

func TestAllCandidatesGapTolerant_DropsSingletons(t *testing.T) {
	t.Parallel()
	intro := map[uint64]bool{0xAA: true}
	target := []uint64{0x00, 0xAA, 0x00, 0x00, 0x00, 0xAA, 0xAA}
	ms := msSeq(7, 10_000)
	cands := allCandidatesGapTolerant(intro, target, ms, 0)
	require.Len(t, cands, 1)
	assert.Equal(t, int64(50_000), cands[0].startMs)
	assert.Equal(t, int64(60_000), cands[0].endMs)
}

func TestAllCandidatesGapTolerant_UsesRealJitteredTimestamps(t *testing.T) {
	t.Parallel()
	// Simulates skip_frame nokey: ffmpeg snapped frames to keyframes, so
	// timestamps are NOT uniform multiples of the requested interval.
	intro := map[uint64]bool{0xAA: true, 0xBB: true, 0xCC: true}
	target := []uint64{0x00, 0xAA, 0xBB, 0xCC, 0x00}
	// Realistic: intended interval 15s, actual frames at 0, 13.4, 28.1, 42.6, 57.2
	ms := []int64{0, 13_400, 28_100, 42_600, 57_200}
	cands := allCandidatesGapTolerant(intro, target, ms, 0)
	require.Len(t, cands, 1)
	assert.Equal(t, int64(13_400), cands[0].startMs)
	assert.Equal(t, int64(57_200), cands[0].endMs) // frame 4 ms = next-frame end
}

// --- classifyConfirmed tests ---

func TestClassifyConfirmed_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, classifyConfirmed(nil))
}

func TestClassifyConfirmed_PicksLongestAsIntro(t *testing.T) {
	t.Parallel()
	wins := []confirmedWindow{
		{startMs: 0, endMs: 30_000, confidence: 0.9},
		{startMs: 60_000, endMs: 180_000, confidence: 0.95}, // longest
		{startMs: 200_000, endMs: 220_000, confidence: 0.9},
	}
	segs := classifyConfirmed(wins)
	// Should produce: recap (0-30s, <60s, before intro) + intro (60-180s)
	require.Len(t, segs, 2)
	assert.Equal(t, "recap", segs[0].Type)
	assert.Equal(t, int64(0), segs[0].StartMs)
	assert.Equal(t, int64(30_000), segs[0].EndMs)
	assert.Equal(t, "intro", segs[1].Type)
	assert.Equal(t, int64(60_000), segs[1].StartMs)
	assert.Equal(t, int64(180_000), segs[1].EndMs)
	assert.Equal(t, 0.95, segs[1].Confidence)
}

func TestClassifyConfirmed_NoRecapWhenEarlierIsLong(t *testing.T) {
	t.Parallel()
	wins := []confirmedWindow{
		{startMs: 0, endMs: 90_000, confidence: 0.9}, // ≥60s, not recap
		{startMs: 120_000, endMs: 200_000, confidence: 0.9},
	}
	segs := classifyConfirmed(wins)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
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
func (f *fakeDetector) AudioBandedFingerprint(_ context.Context, _ string, _, _ float64, _ int) ([]float64, error) {
	return nil, nil
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

// writeTrickplaySidecarFixture writes a minimal sidecar JSON for a given media
// id under cacheDir so extractEpisodeHashes can read frame timestamps.
func writeTrickplaySidecarFixture(t *testing.T, cacheDir, mediaID string, intervalSec, framesPerSprite, totalFrames int) {
	t.Helper()
	dir := cacheDir + "/" + mediaID
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	frames := make([]int64, totalFrames)
	for i := range totalFrames {
		frames[i] = int64(i*intervalSec) * 1000
	}
	payload := fmt.Sprintf(`{"tile_width":5,"tile_height":3,"frames_per_sprite":%d,"frame_ms":[%s]}`,
		framesPerSprite, joinInts64(frames))
	path := dir + "/trickplay.json"
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func joinInts64(s []int64) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%d", v)
	}
	return out
}

// newTestServiceWithTrickplay assembles an IntroDetectionService with a
// manifest-returning trickplay repo and a per-test temp cache dir + sidecar.
func newTestServiceWithTrickplay(t *testing.T, det IntroDetector, segRepo domainlibrary.MediaSegmentRepository, item *domainlibrary.MediaItem, manifest *domainlibrary.TrickplayManifest) *IntroDetectionService {
	t.Helper()
	cacheDir := t.TempDir()
	totalFrames := manifest.ImageCount * manifest.TileWidth * manifest.TileHeight
	writeTrickplaySidecarFixture(t, cacheDir, string(manifest.MediaItemID), manifest.Interval, manifest.TileWidth*manifest.TileHeight, totalFrames)
	return NewIntroDetectionService(
		segRepo,
		&fakeTrickplayRepoWithManifest{manifest: manifest},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		det,
		cacheDir,
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
	svc := newTestServiceWithTrickplay(t, det, segRepo, item, manifest)

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
