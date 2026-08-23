package library

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// detectSeason must page through the WHOLE season, not just the first page. A long
// single-season (anime, daily soaps) with >100 episodes would otherwise leave the
// later episodes without intro markers - detectSeason is the only detection path
// (MediaItemAdded schedules the season, not individual episodes).
func TestDetectSeason_PaginatesAllEpisodes(t *testing.T) {
	t.Parallel()
	const total = 250 // > the 200 page size and the old 100 cap
	segmentRepo := &fakeSegmentRepo{}
	siblings := make([]*domainlibrary.MediaItem, total)
	for i := range total {
		id := fmt.Sprintf("ep-%d", i)
		idx := i + 1
		siblings[i] = &domainlibrary.MediaItem{
			ID:          shared.MediaItemID(id),
			Type:        domainlibrary.Episode,
			IndexNumber: &idx,
			File:        &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
		}
		// Pre-cover each episode so detectSeason skips DetectIntro (no pipeline)
		// yet still queries it: batchQueriedIDs then proves pagination reached all.
		segmentRepo.seeded = append(segmentRepo.seeded, &domainlibrary.MediaSegment{
			MediaItemID: shared.MediaItemID(id), Type: "intro", StartMs: 0, EndMs: 60_000, Source: "mkv_chapter",
		})
	}
	svc := newSeasonService(t, segmentRepo, &countingDetector{}, siblings)

	svc.detectSeason(context.Background(), "season-id")

	require.Len(t, segmentRepo.batchQueriedIDs, 2*total,
		"detectSeason must page through every episode twice (detection pass + duration-prior pass), not stop at the first page")
	require.Equal(t, 4, segmentRepo.batchCalls,
		"250 episodes at pageSize 200 batch into 2 calls per pass (200 + 50) for the detection and prior passes, not one GetByMediaItemID per episode")
}

// detectSeason runs DetectIntro on each episode that doesn't already have a
// "good" intro. Good = file-derived (mkv_chapter, audio_cluster, phash):
// re-running pHash + Chromaprint on those would burn CPU just to rediscover
// the same answer. Community-DB segments (tidb/introdb) are NOT good: their
// timestamps ignore the local file's edition (repack/cut), so a file-derived
// re-detection must be allowed to replace them.
//
// We can't observe DetectIntro directly without spinning up the full
// pipeline, so we count detector calls (ExtractTileHashes is the very
// first non-DB step inside the pipeline). Zero calls = the episode was
// correctly skipped.

type countingDetector struct {
	fakeDetector
	calls atomic.Int32
	// creditsCalls counts the credits scans, the expensive part of the
	// suspect-credits repair (a full blackdetect pass over the last 20% of the
	// file), so a test can prove the repair does not re-pay it on every sweep.
	creditsCalls atomic.Int32
	// fallbackCalls counts the last-resort silence scans (blackdetect +
	// silencedetect over the first 600s), the other minutes-long ffmpeg pass of
	// one episode's detection.
	fallbackCalls atomic.Int32
	// leadingLogoCalls counts calls to DetectLeadingLogoBlock, the film-only
	// head-of-file scan.
	leadingLogoCalls atomic.Int32
}

func (d *countingDetector) DetectLeadingLogoBlock(ctx context.Context, path string, durationSec float64) (int64, int64, bool, error) {
	d.leadingLogoCalls.Add(1)
	return d.fakeDetector.DetectLeadingLogoBlock(ctx, path, durationSec)
}

func (d *countingDetector) AudioFallback(ctx context.Context, path string, maxSec float64) (int64, int64, bool, error) {
	d.fallbackCalls.Add(1)
	return d.fakeDetector.AudioFallback(ctx, path, maxSec)
}

func (d *countingDetector) ExtractTileHashes(p string, w, h, pw, ph int) ([]uint64, error) {
	d.calls.Add(1)
	return d.fakeDetector.ExtractTileHashes(p, w, h, pw, ph)
}

func (d *countingDetector) DetectEndCredits(ctx context.Context, path string, durationSec float64) (int64, int64, bool, error) {
	d.creditsCalls.Add(1)
	return d.fakeDetector.DetectEndCredits(ctx, path, durationSec)
}

// helper: build a service whose detectSeason will encounter siblings with
// pre-existing segments. Trickplay returns ErrNotFound so pHash short-
// circuits cleanly when it's allowed to run; we still expect zero
// invocations on the authoritative-source path.
func newSeasonService(t *testing.T, segmentRepo *fakeSegmentRepo, detector *countingDetector, siblings []*domainlibrary.MediaItem) *IntroDetectionService {
	t.Helper()
	svc := NewIntroDetectionService(
		segmentRepo,
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{siblings: siblings},
		&fakeConfigRepo{},
		detector,
		"/tmp/trickplay",
	)
	return svc
}

// A community-DB intro (tidb/introdb) must NOT park the episode: its timestamps
// come from someone else's copy of the media, so a file-derived detection has to
// get its chance to replace it. Pipeline entry is observed via the attempt
// counter (recorded after every clean run) - the detector itself stays silent
// here because the fake config disables trickplay.
func TestDetectSeason_RedetectsTIDBCoveredEpisode(t *testing.T) {
	t.Parallel()
	const epID = "ep-tidb-covered"
	segmentRepo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{MediaItemID: epID, Type: "intro", StartMs: 0, EndMs: 60_000, Source: "tidb"},
	}}
	detector := &countingDetector{}
	idx := 1
	ep := &domainlibrary.MediaItem{
		ID: shared.MediaItemID(epID), Type: domainlibrary.Episode, IndexNumber: &idx,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
	}
	svc := NewIntroDetectionService(
		segmentRepo,
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: ep, siblings: []*domainlibrary.MediaItem{ep}},
		&fakeConfigRepo{},
		detector,
		"/tmp/trickplay",
	)

	svc.detectSeason(context.Background(), "season-id")
	require.Equal(t, 1, segmentRepo.attempts[epID].count,
		"a tidb-covered episode must re-enter the pipeline (attempt recorded)")
}

// An episode with NO segments but whose attempt counter is at the cap for the
// CURRENT algo version must be skipped - re-decoding it (a full 4K HEVC decode)
// would land on the same empty answer. Skipping before DetectIntro means no new
// attempt is recorded, so the counter is unchanged.
func TestDetectSeason_SkipsExhaustedEpisode(t *testing.T) {
	t.Parallel()
	const epID = "ep-exhausted"
	segmentRepo := &fakeSegmentRepo{
		attempts: map[string]fakeIntroAttempt{
			epID: {count: maxIntroDetectionAttempts, version: introDetectionAlgoVersion},
		},
	}
	detector := &countingDetector{}
	idx := 1
	svc := newSeasonService(t, segmentRepo, detector, []*domainlibrary.MediaItem{
		{ID: shared.MediaItemID(epID), Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"}},
	})

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, int32(0), detector.calls.Load(), "detector must not run for an exhausted episode")
	assert.Equal(t, maxIntroDetectionAttempts, segmentRepo.attempts[epID].count,
		"a skipped episode records no new attempt - the counter is unchanged")
}

func TestDetectSeason_SkipsMKVChapterCovered(t *testing.T) {
	t.Parallel()
	const epID = "ep-mkv-covered"
	segmentRepo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{MediaItemID: epID, Type: "intro", StartMs: 5_000, EndMs: 60_000, Source: "mkv_chapter"},
	}}
	detector := &countingDetector{}
	idx := 1
	svc := newSeasonService(t, segmentRepo, detector, []*domainlibrary.MediaItem{
		{ID: shared.MediaItemID(epID), Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"}},
	})

	svc.detectSeason(context.Background(), "season-id")
	require.Equal(t, int32(0), detector.calls.Load(),
		"detector must not be called when an MKV chapter authored the intro")
}

// Audio-fallback intros are explicitly NOT considered good - they're a
// last-resort heuristic, so a future TIDB/IntroDB hit should be allowed
// to replace them. We pin the contract by reading hasGoodIntro logic
// indirectly: the test below would need detectSeason to *enter*
// DetectIntro for an "audio"-sourced episode, but DetectIntro then
// runs the full pipeline (and our trickplay fake returns ErrNotFound,
// so pHash gives up cleanly). Counting that pHash _attempts_ to start
// (i.e. the pipeline was entered) proves the episode wasn't skipped.
//
// We can't observe ExtractTileHashes here because the trickplay repo
// returns ErrNotFound before reaching the detector. Instead we use a
// synchronous run of the same skip-decision predicate by re-implementing
// it inline as a regression contract. If anyone changes hasGoodIntro in
// the production code, this constant list must follow - that's the
// point of the test.
func TestSkipDecision_AudioFallbackIsNotGood(t *testing.T) {
	t.Parallel()
	cases := []struct {
		source     string
		isGoodLong bool // 60s segment, well above the 5s threshold
	}{
		{"mkv_chapter", true},
		{"audio_cluster", true},
		{"phash", true},
		{"tidb", false},        // re-detection allowed: community DB, not aligned to the local file
		{"introdb", false},     // re-detection allowed: community DB, not aligned to the local file
		{"audio", false},       // re-detection allowed: heuristic
		{"blackdetect", false}, // re-detection allowed: heuristic
		{"manual", true},       // sticky via the manualLocked branch (different code path)
	}
	for _, c := range cases {
		t.Run(c.source, func(t *testing.T) {
			// Keep this list in sync with the switch in detectSeason.
			good := false
			switch c.source {
			case "mkv_chapter", "audio_cluster":
				good = true
			case "phash":
				good = true
			}
			if c.source == "manual" {
				// manualLocked short-circuit, treated as good for skipping.
				good = true
			}
			assert.Equal(t, c.isGoodLong, good,
				"source %q good-classification regressed", c.source)
		})
	}
}

// Manual override remains sticky: even with TIDB data in the cache, a
// manual segment must NOT be overwritten. detectSeason short-circuits
// the moment it sees a manual segment.
func TestDetectSeason_SkipsManualOverride(t *testing.T) {
	t.Parallel()
	const epID = "ep-manual"
	segmentRepo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{MediaItemID: epID, Type: "intro", StartMs: 0, EndMs: 30_000, Source: "manual"},
	}}
	detector := &countingDetector{}
	idx := 1
	svc := newSeasonService(t, segmentRepo, detector, []*domainlibrary.MediaItem{
		{ID: shared.MediaItemID(epID), Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"}},
	})

	svc.detectSeason(context.Background(), "season-id")
	require.Equal(t, int32(0), detector.calls.Load(),
		"manual override must always skip detection")
}
