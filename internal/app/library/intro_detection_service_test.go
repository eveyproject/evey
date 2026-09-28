package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefineEndBySubtitleOnset(t *testing.T) {
	t.Parallel()
	newSvc := func(cues []int64) *IntroDetectionService {
		return &IntroDetectionService{detector: &fakeDetector{subtitleCues: cues}}
	}
	const start, audioEnd int64 = 260_000, 349_000 // music-bleed end, +34s too late

	// Dialogue resumes at 315s → end pulled in from 349s to 315s.
	if got := newSvc([]int64{12_000, 250_000, 315_000, 360_000}).
		refineEndBySubtitleOnset(context.Background(), "f.mkv", start, audioEnd); got != 315_000 {
		t.Errorf("music-bleed: end = %d, want 315000", got)
	}
	// A cold-open line lingering at the theme onset (within guard) is skipped;
	// the next cue (320s) is the real resume.
	if got := newSvc([]int64{262_000, 320_000}).
		refineEndBySubtitleOnset(context.Background(), "f.mkv", start, audioEnd); got != 320_000 {
		t.Errorf("guard skip: end = %d, want 320000", got)
	}
	// No subtitles → no-op (audio end kept). Media without subs is unaffected.
	if got := newSvc(nil).
		refineEndBySubtitleOnset(context.Background(), "f.mkv", start, audioEnd); got != audioEnd {
		t.Errorf("no subs: end = %d, want %d", got, audioEnd)
	}
	// First post-guard cue past the audio end (silent post-intro scene) → not
	// trusted, keep the audio end rather than overshoot far.
	if got := newSvc([]int64{600_000}).
		refineEndBySubtitleOnset(context.Background(), "f.mkv", start, audioEnd); got != audioEnd {
		t.Errorf("implausible cue: end = %d, want %d", got, audioEnd)
	}
}

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

// msSeq builds a slice of timestamps with constant spacing - convenience for
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
	assert.Empty(t, classifyConfirmed(nil, defaultIntroConfig()))
}

func TestClassifyConfirmed_PicksLongestAsIntro(t *testing.T) {
	t.Parallel()
	wins := []confirmedWindow{
		{startMs: 0, endMs: 30_000, confidence: 0.9},
		{startMs: 60_000, endMs: 180_000, confidence: 0.95}, // longest
		{startMs: 200_000, endMs: 220_000, confidence: 0.9},
	}
	segs := classifyConfirmed(wins, defaultIntroConfig())
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

// With TWO short windows before the intro (a confirmed recurring bumper early,
// the real "previously on" right before the title sequence), the recap is the
// window CLOSEST to the intro - not the first chronological match.
func TestClassifyConfirmed_RecapIsClosestWindowToIntro(t *testing.T) {
	t.Parallel()
	wins := []confirmedWindow{
		{startMs: 0, endMs: 15_000, confidence: 0.9},        // network bumper
		{startMs: 100_000, endMs: 140_000, confidence: 0.9}, // the actual recap
		{startMs: 150_000, endMs: 260_000, confidence: 0.95},
	}
	segs := classifyConfirmed(wins, defaultIntroConfig())
	require.Len(t, segs, 2)
	assert.Equal(t, "recap", segs[0].Type)
	assert.Equal(t, int64(100_000), segs[0].StartMs, "the recap is the qualifying window closest to the intro")
	assert.Equal(t, int64(140_000), segs[0].EndMs)
	assert.Equal(t, "intro", segs[1].Type)
}

func TestClassifyConfirmed_NoRecapWhenEarlierIsLong(t *testing.T) {
	t.Parallel()
	wins := []confirmedWindow{
		{startMs: 0, endMs: 90_000, confidence: 0.9}, // ≥60s, not recap
		{startMs: 120_000, endMs: 200_000, confidence: 0.9},
	}
	segs := classifyConfirmed(wins, defaultIntroConfig())
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
}

// --- end credits service integration tests ---

// fakeDetector implements IntroDetector for tests.
// ExtractTileHashes, PrecisionPass, AudioFallback return zero/not-found;
// DetectEndCredits and DetectStudioLogo return the configured responses.
type fakeDetector struct {
	// thumbs answers GrayThumbnails; nil decodes nothing, so no visual widening.
	thumbs func(path string, startSec, durationSec, fps float64) [][]byte

	creditsStartMs int64
	creditsEndMs   int64
	creditsFound   bool
	creditsErr     error

	logoStartMs int64
	logoEndMs   int64
	logoFound   bool

	// precision* is what PrecisionPass answers; zero values keep the echo.
	precisionStartMs int64
	precisionEndMs   int64
	precisionErr     error

	// leadingLogo* is what DetectLeadingLogoBlock answers.
	leadingLogoStartMs int64
	leadingLogoEndMs   int64
	leadingLogoFound   bool
	leadingLogoErr     error

	// fallback* is what AudioFallback answers (the last-resort silence pass).
	fallbackStartMs int64
	fallbackEndMs   int64
	fallbackFound   bool
	// fallbackErr makes the silence pass FAIL. It is the one detector error the
	// pipeline propagates to the caller, so it is the only way a test can reach
	// the sweep's failure branch - and it is the pass that OOMed in prod.
	fallbackErr error

	// tileHashes is returned by ExtractTileHashes when non-nil; defaults to nil (no hashes).
	tileHashes []uint64

	// subtitleCues is returned by SubtitleCueStarts (nil = no subtitles).
	subtitleCues []int64
	// sceneCuts is returned by SceneCutTimestamps (nil = none).
	sceneCuts []int64
}

func (f *fakeDetector) ExtractTileHashes(_ string, _, _, _, _ int) ([]uint64, error) {
	return f.tileHashes, nil
}

// PrecisionPass echoes its input by default, which the pipeline reads as
// "found nothing" - that is the historical behaviour and every existing test
// depends on it. precisionStartMs/precisionEndMs let a test answer something
// ELSE, which is the only way to reach the refined branch of
// refineWithPrecisionPass: with both fakes echoing, that branch was
// unreachable from any test in the repo.
func (f *fakeDetector) PrecisionPass(_ context.Context, _ string, startSec, endSec, _ float64) (int64, int64, error) {
	if f.precisionErr != nil {
		return 0, 0, f.precisionErr
	}
	if f.precisionStartMs != 0 || f.precisionEndMs != 0 {
		return f.precisionStartMs, f.precisionEndMs, nil
	}
	return int64(startSec * 1000), int64(endSec * 1000), nil
}
func (f *fakeDetector) AudioFallback(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	if f.fallbackErr != nil {
		return 0, 0, false, f.fallbackErr
	}
	return f.fallbackStartMs, f.fallbackEndMs, f.fallbackFound, nil
}
func (f *fakeDetector) DetectEndCredits(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	return f.creditsStartMs, f.creditsEndMs, f.creditsFound, f.creditsErr
}
func (f *fakeDetector) DetectStudioLogo(_ []uint64, _ int) (int64, int64, bool) {
	return f.logoStartMs, f.logoEndMs, f.logoFound
}
func (f *fakeDetector) DetectLeadingLogoBlock(_ context.Context, _ string, _ float64) (int64, int64, bool, error) {
	return f.leadingLogoStartMs, f.leadingLogoEndMs, f.leadingLogoFound, f.leadingLogoErr
}
func (f *fakeDetector) AudioBandedFingerprint(_ context.Context, _ string, _, _ float64, _ int) ([]float64, error) {
	return nil, nil
}
func (f *fakeDetector) SubtitleCueStarts(_ context.Context, _ string) ([]int64, error) {
	return f.subtitleCues, nil
}
func (f *fakeDetector) SceneCutTimestamps(_ context.Context, _ string, _, _, _ float64) ([]int64, error) {
	return f.sceneCuts, nil
}
func (f *fakeDetector) GrayThumbnails(_ context.Context, path string, startSec, durationSec, fps float64) ([][]byte, error) {
	if f.thumbs == nil {
		return nil, nil
	}
	return f.thumbs(path, startSec, durationSec, fps), nil
}

// fakeSegmentRepo records all Upsert calls and stores segments by (mediaItemID, type).
// GetByMediaItemID returns segments that were previously upserted.
type fakeSegmentRepo struct {
	// mu makes the fake safe under the sweep's worker pool, like the database
	// it stands in for. Without it the fake was only ever correct for a
	// SEQUENTIAL sweep, and forEachBackfillTarget has fanned out for a while.
	mu       sync.Mutex
	upserted []*domainlibrary.MediaSegment
	// seeded allows tests to pre-populate segments for GetByMediaItemID.
	seeded []*domainlibrary.MediaSegment
	// deleted records (mediaItemID, type) pairs passed to DeleteByMediaItemIDAndType.
	deleted []string
	// queriedIDs records every id passed to GetByMediaItemID (one per episode
	// detectSeason reaches) so a test can prove it paged through them all.
	queriedIDs []string
	// batchCalls counts calls to GetByMediaItemIDs, proving detectSeason
	// issues one batched call per page instead of one GetByMediaItemID call
	// per episode.
	batchCalls int
	// batchQueriedIDs accumulates every id passed to GetByMediaItemIDs across
	// ALL calls, so a test can prove every episode was covered (batched
	// per-page, not per-episode).
	batchQueriedIDs []string
	// getErr makes every GetByMediaItemID call fail with this error.
	getErr error
	// getErrForID makes GetByMediaItemID fail for specific IDs only.
	getErrForID map[string]error
	// batchErr makes every GetByMediaItemIDs call fail with this error.
	batchErr error
	// upsertErr makes every UpsertUnlessManual call fail with this error.
	upsertErr error
	// attempts tracks the intro-detection attempt counter per media id, mirroring
	// the postgres version-gated upsert.
	attempts map[string]fakeIntroAttempt
	// exhaustedBatchCalls counts calls to IntroDetectionExhaustedBatch, proving
	// detectSeason issues one batched call per page instead of one per candidate
	// episode.
	exhaustedBatchCalls int
	// exhaustedBatchErr makes every IntroDetectionExhaustedBatch call fail with
	// this error.
	exhaustedBatchErr error
	// pendingCalls counts ListSeasonsPendingIntroDetection calls.
	pendingCalls int
	// episodesBySeason maps a season parent id to its episode ids. It is what
	// lets ListSeasonsPendingIntroDetection be DERIVED from the fake's own rows
	// the way the SQL derives it from the tables - a fake handing back a canned
	// list would prove nothing about a sweep resuming from the database.
	episodesBySeason map[string][]shared.MediaItemID
	// onPendingListed fires inside ListSeasonsPendingIntroDetection with the
	// number of ids it is about to return, so a test can interrupt or block a
	// sweep exactly where a redeployment would.
	onPendingListed func(int)
	// dropAttempts makes RecordIntroDetectionAttempt a no-op, simulating seasons
	// the sweep walks without making any progress.
	dropAttempts bool
	// ops is the ordered write/delete log replayed by the read methods. The real
	// table is UNIQUE on (media_item_id, type): an upsert REPLACES the row and a
	// delete removes it. A fake that only appended returned two rows for one key
	// and kept serving deleted ones, which hides every read-after-write bug.
	ops []fakeSegOp
}

// fakeSegOp is one recorded write (seg != nil) or delete (seg == nil, typ ""
// meaning every type of the item).
type fakeSegOp struct {
	seg *domainlibrary.MediaSegment
	id  shared.MediaItemID
	typ string
}

// replay applies the op log on top of the seeded rows for one media item.
func (r *fakeSegmentRepo) replay(id shared.MediaItemID) []*domainlibrary.MediaSegment {
	byType := map[string]*domainlibrary.MediaSegment{}
	var order []string
	put := func(s *domainlibrary.MediaSegment) {
		if _, ok := byType[s.Type]; !ok {
			order = append(order, s.Type)
		}
		byType[s.Type] = s
	}
	for _, s := range r.seeded {
		if s.MediaItemID == id {
			put(s)
		}
	}
	for _, op := range r.ops {
		if op.id != id {
			continue
		}
		switch {
		case op.seg != nil:
			put(op.seg)
		case op.typ == "":
			byType = map[string]*domainlibrary.MediaSegment{}
		default:
			delete(byType, op.typ)
		}
	}
	out := make([]*domainlibrary.MediaSegment, 0, len(order))
	for _, t := range order {
		if s, ok := byType[t]; ok {
			out = append(out, s)
		}
	}
	return out
}

// manualLocked reports whether the CURRENT row for (id, type) is an operator
// mark. operatorOnly excludes 'manual_propagated', matching the two conditional
// upserts of the real repository.
func (r *fakeSegmentRepo) manualLocked(seg *domainlibrary.MediaSegment, operatorOnly bool) bool {
	for _, e := range r.replay(seg.MediaItemID) {
		if e.Type != seg.Type {
			continue
		}
		if e.Source == "manual" || (!operatorOnly && e.Source == "manual_propagated") {
			return true
		}
	}
	return false
}

type fakeIntroAttempt struct {
	count   int
	version int
}

func (r *fakeSegmentRepo) Upsert(_ context.Context, seg *domainlibrary.MediaSegment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserted = append(r.upserted, seg)
	r.ops = append(r.ops, fakeSegOp{seg: seg, id: seg.MediaItemID, typ: seg.Type})
	return nil
}

// UpsertUnlessManual mirrors the postgres conditional upsert: it skips (returns
// applied=false) when an existing seeded/upserted segment for the same
// (mediaItemID, type) is a manual mark, else records the upsert.
func (r *fakeSegmentRepo) UpsertUnlessManual(_ context.Context, seg *domainlibrary.MediaSegment) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.upsertErr != nil {
		return false, r.upsertErr
	}
	if r.manualLocked(seg, false) {
		return false, nil
	}
	r.upserted = append(r.upserted, seg)
	r.ops = append(r.ops, fakeSegOp{seg: seg, id: seg.MediaItemID, typ: seg.Type})
	return true, nil
}

// UpsertUnlessOperatorManual mirrors the postgres operator-only guard: it skips
// (applied=false) only when an existing segment is a hard 'manual' mark;
// 'manual_propagated' rows ARE overwritten (a re-run refreshes its own work).
func (r *fakeSegmentRepo) UpsertUnlessOperatorManual(_ context.Context, seg *domainlibrary.MediaSegment) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.upsertErr != nil {
		return false, r.upsertErr
	}
	if r.manualLocked(seg, true) {
		return false, nil
	}
	r.upserted = append(r.upserted, seg)
	r.ops = append(r.ops, fakeSegOp{seg: seg, id: seg.MediaItemID, typ: seg.Type})
	return true, nil
}
func (r *fakeSegmentRepo) GetByMediaItemID(_ context.Context, id shared.MediaItemID) ([]*domainlibrary.MediaSegment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queriedIDs = append(r.queriedIDs, string(id))
	if r.getErr != nil {
		return nil, r.getErr
	}
	if err, ok := r.getErrForID[string(id)]; ok {
		return nil, err
	}
	return r.replay(id), nil
}

// GetByMediaItemIDs mirrors the postgres batch query: one call covering every
// requested id, grouped into a map. Missing ids are absent from the map.
func (r *fakeSegmentRepo) GetByMediaItemIDs(_ context.Context, ids []shared.MediaItemID) (map[shared.MediaItemID][]*domainlibrary.MediaSegment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batchCalls++
	for _, id := range ids {
		r.batchQueriedIDs = append(r.batchQueriedIDs, string(id))
	}
	if r.batchErr != nil {
		return nil, r.batchErr
	}
	out := make(map[shared.MediaItemID][]*domainlibrary.MediaSegment, len(ids))
	for _, id := range ids {
		if segs := r.replay(id); len(segs) > 0 {
			out[id] = segs
		}
	}
	return out, nil
}

func (r *fakeSegmentRepo) DeleteByMediaItemID(_ context.Context, id shared.MediaItemID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops = append(r.ops, fakeSegOp{id: id})
	return nil
}
func (r *fakeSegmentRepo) DeleteByMediaItemIDAndType(_ context.Context, id shared.MediaItemID, segType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, string(id)+"/"+segType)
	r.ops = append(r.ops, fakeSegOp{id: id, typ: segType})
	return nil
}

// ListSeasonsPendingIntroDetection mirrors the postgres predicate: a season is
// returned when at least one of its episodes has no settled intro AND has not
// burned its retry budget at algoVersion. Derived from the fake's own rows and
// attempt counters, so it SHRINKS as detection progresses exactly like the SQL -
// which is the whole property the resumable sweep relies on. exclude is the
// caller's in-run skip set and limit bounds the batch; neither goes through the
// paginated List path, so DefaultListLimit does not apply here.
func (r *fakeSegmentRepo) ListSeasonsPendingIntroDetection(_ context.Context, algoVersion, maxAttempts, limit int, exclude []string) ([]string, error) {
	out, hook := r.pendingSeasons(algoVersion, maxAttempts, limit, exclude)
	// Outside the lock: tests use this hook to block a sweep mid-flight, and
	// holding the repo lock there would freeze every other caller too.
	if hook != nil {
		hook(len(out))
	}
	return out, nil
}

func (r *fakeSegmentRepo) pendingSeasons(algoVersion, maxAttempts, limit int, exclude []string) ([]string, func(int)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingCalls++
	seasons := make([]string, 0, len(r.episodesBySeason))
	for season := range r.episodesBySeason {
		seasons = append(seasons, season)
	}
	sort.Strings(seasons) // the query is ORDER BY parent_id
	var out []string
	for _, season := range seasons {
		if slices.Contains(exclude, season) {
			continue
		}
		for _, ep := range r.episodesBySeason[season] {
			if a, ok := r.attempts[string(ep)]; ok && a.version == algoVersion && a.count >= maxAttempts {
				continue
			}
			settled := false
			for _, sg := range r.replay(ep) {
				a, attempted := r.attempts[string(ep)]
				if sg.Type == "intro" && domainlibrary.IntroDetectionSettled(sg.Source, sg.EndMs-sg.StartMs, attempted && a.version == algoVersion) {
					settled = true
					break
				}
			}
			if !settled {
				out = append(out, season)
				break
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, r.onPendingListed
}

func (r *fakeSegmentRepo) RecordIntroDetectionAttempt(_ context.Context, mediaItemID string, algoVersion int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dropAttempts {
		return nil
	}
	if r.attempts == nil {
		r.attempts = map[string]fakeIntroAttempt{}
	}
	a := r.attempts[mediaItemID]
	if a.version == algoVersion {
		a.count++
	} else {
		a.count = 1
		a.version = algoVersion
	}
	r.attempts[mediaItemID] = a
	return nil
}

// IntroDetectionAttemptVersions mirrors the postgres query: last version per
// attempted item, absent otherwise.
func (r *fakeSegmentRepo) IntroDetectionAttemptVersions(_ context.Context, mediaItemIDs []string) (map[string]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if a, ok := r.attempts[id]; ok {
			out[id] = a.version
		}
	}
	return out, nil
}

// IntroDetectionExhaustedBatch mirrors the postgres batch query: one call
// covering every requested id, returning a map where a not-exhausted id is
// simply absent (not false-valued).
func (r *fakeSegmentRepo) IntroDetectionExhaustedBatch(_ context.Context, mediaItemIDs []string, algoVersion, maxAttempts int) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exhaustedBatchCalls++
	if r.exhaustedBatchErr != nil {
		return nil, r.exhaustedBatchErr
	}
	out := make(map[string]bool, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		a, ok := r.attempts[id]
		if ok && a.version == algoVersion && a.count >= maxAttempts {
			out[id] = true
		}
	}
	return out, nil
}

// fakeMediaItemRepo returns the configured item for GetByID calls and optionally
// a list of siblings for List calls. Intro detection reads nothing else, so the
// rest of the query port is left to the embedded nil interface.
type fakeMediaItemRepo struct {
	domainlibrary.MediaItemQueryRepo
	item     *domainlibrary.MediaItem
	siblings []*domainlibrary.MediaItem
}

// GetByID answers the configured single item when there is one, else looks the
// id up among the siblings. A fake that answered the SAME item to every id made
// a multi-season sweep look like it worked while every episode resolved to the
// same file.
func (r *fakeMediaItemRepo) GetByID(_ context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	if r.item != nil {
		return r.item, nil
	}
	for _, it := range r.siblings {
		if it.ID == id {
			return it, nil
		}
	}
	return nil, shared.ErrNotFound
}
func (r *fakeMediaItemRepo) List(_ context.Context, f domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	// Honour Offset/Limit so paginating callers (detectSeason) terminate and
	// page correctly - a fake that ignored them would loop forever once the
	// caller pages past a full page. Honour Type so callers that enumerate
	// seasons vs movies vs episodes (BackfillLibrary) see distinct sets.
	items := r.siblings
	// Honour ParentID when at least one fixture declares one: a sweep walking
	// several seasons must not receive every episode of the catalogue for each
	// of them. Fixtures that leave ParentID empty keep the old "one anonymous
	// season" behaviour.
	if f.ParentID != nil && slices.ContainsFunc(items, func(it *domainlibrary.MediaItem) bool { return it.ParentID != "" }) {
		var scoped []*domainlibrary.MediaItem
		for _, it := range items {
			if it.ParentID == *f.ParentID {
				scoped = append(scoped, it)
			}
		}
		items = scoped
	}
	if f.Type != nil {
		var typed []*domainlibrary.MediaItem
		for _, it := range items {
			if string(it.Type) == *f.Type {
				typed = append(typed, it)
			}
		}
		items = typed
	}
	if f.Offset >= len(items) {
		return nil, nil
	}
	if f.Offset > 0 {
		items = items[f.Offset:]
	}
	if f.Limit > 0 && f.Limit < len(items) {
		items = items[:f.Limit]
	}
	return items, nil
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

// GetByMediaItemIDs mirrors GetByMediaItemID's "nothing found" behaviour: every
// requested id is simply absent from the returned map.
func (r *fakeTrickplayRepo) GetByMediaItemIDs(_ context.Context, _ []shared.MediaItemID) (map[shared.MediaItemID]*domainlibrary.TrickplayManifest, error) {
	return map[shared.MediaItemID]*domainlibrary.TrickplayManifest{}, nil
}
func (r *fakeTrickplayRepo) ListMissingMediaItemIDs(_ context.Context, _ int) ([]string, error) {
	return nil, nil
}
func (r *fakeTrickplayRepo) Create(_ context.Context, _ *domainlibrary.TrickplayManifest) error {
	return nil
}
func (r *fakeTrickplayRepo) Delete(_ context.Context, _ shared.MediaItemID) error { return nil }
func (r *fakeTrickplayRepo) RecordFailure(_ context.Context, _ shared.MediaItemID, _ int64, _ time.Time, _ string) error {
	return nil
}

func (r *fakeTrickplayRepoWithManifest) GetByMediaItemID(_ context.Context, _ shared.MediaItemID) (*domainlibrary.TrickplayManifest, error) {
	return r.manifest, nil
}

// GetByMediaItemIDs mirrors GetByMediaItemID's unconditional-hit behaviour:
// every requested id maps to the same fixture manifest.
func (r *fakeTrickplayRepoWithManifest) GetByMediaItemIDs(_ context.Context, ids []shared.MediaItemID) (map[shared.MediaItemID]*domainlibrary.TrickplayManifest, error) {
	out := make(map[shared.MediaItemID]*domainlibrary.TrickplayManifest, len(ids))
	for _, id := range ids {
		out[id] = r.manifest
	}
	return out, nil
}
func (r *fakeTrickplayRepoWithManifest) ListMissingMediaItemIDs(_ context.Context, _ int) ([]string, error) {
	return nil, nil
}
func (r *fakeTrickplayRepoWithManifest) Create(_ context.Context, _ *domainlibrary.TrickplayManifest) error {
	return nil
}
func (r *fakeTrickplayRepoWithManifest) Delete(_ context.Context, _ shared.MediaItemID) error {
	return nil
}
func (r *fakeTrickplayRepoWithManifest) RecordFailure(_ context.Context, _ shared.MediaItemID, _ int64, _ time.Time, _ string) error {
	return nil
}

// fakeConfigRepo disables trickplay so the pHash pass is skipped immediately,
// preventing the 24h waitForManifest loop from blocking tests. values overrides
// or adds entries per key.
//
// It is KEYED, like the real table: the previous version answered the
// trickplay entry to EVERY key, so a test could not set intro_detection.config
// at all and loadIntroConfig silently fell back to defaults on a parse error -
// exactly the shape of fake that keeps a config-driven branch untested.
type fakeConfigRepo struct{ values map[string]string }

func (r *fakeConfigRepo) Get(_ context.Context, key string) (*shared.ServerConfigEntry, error) {
	if v, ok := r.values[key]; ok {
		return &shared.ServerConfigEntry{Key: key, Value: v}, nil
	}
	if key == "trickplay.enabled" {
		return &shared.ServerConfigEntry{Key: key, Value: "false"}, nil
	}
	return nil, shared.ErrNotFound
}
func (r *fakeConfigRepo) Set(_ context.Context, _, _ string) error { return nil }
func (r *fakeConfigRepo) Delete(_ context.Context, _ string) error { return nil }
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

// TestDetectSeason_BatchesExistingSegmentsLookup proves detectSeason fetches
// the whole page's existing segments in one GetByMediaItemIDs call instead of
// one GetByMediaItemID call per episode (the N+1 this test guards against).
// Every sibling is seeded with an already-good intro segment so the loop
// skips straight to "continue" for each one: the batch lookup is the only
// segment-repo read this path should perform.
func TestDetectSeason_BatchesExistingSegmentsLookup(t *testing.T) {
	t.Parallel()

	const episodeCount = 5
	parentID := "season-1"
	segRepo := &fakeSegmentRepo{}
	siblings := make([]*domainlibrary.MediaItem, episodeCount)
	for i := range episodeCount {
		id := shared.MediaItemID(fmt.Sprintf("ep-%d", i))
		siblings[i] = &domainlibrary.MediaItem{ID: id, Type: domainlibrary.Episode, ParentID: parentID}
		segRepo.seeded = append(segRepo.seeded, &domainlibrary.MediaSegment{
			MediaItemID: id, Type: "intro", Source: "phash", StartMs: 0, EndMs: 8_000,
		})
	}
	itemRepo := &fakeMediaItemRepo{siblings: siblings}
	svc := newTestServiceFull(&fakeDetector{}, segRepo, itemRepo, &fakeTrickplayRepo{})

	svc.detectSeason(context.Background(), parentID)

	require.Equal(t, 2, segRepo.batchCalls,
		"detectSeason issues one batched call for the detection page plus one for the duration-prior pass - never one per episode")
	require.Len(t, segRepo.batchQueriedIDs, 2*episodeCount,
		"each batched call must cover every episode in the page")
	require.Empty(t, segRepo.queriedIDs,
		"detectSeason must not fall back to per-episode GetByMediaItemID calls")
}

// TestDetectSeason_BatchesExhaustedCheck proves detectSeason issues exactly one
// IntroDetectionExhaustedBatch call per page covering every candidate (the N+1
// this restructure closes), and that an episode already exhausted at the
// current algo version is skipped (no detection run, no new attempt recorded)
// while a non-exhausted sibling still runs detection.
func TestDetectSeason_BatchesExhaustedCheck(t *testing.T) {
	t.Parallel()

	parentID := "season-exhausted"
	exhaustedID := shared.MediaItemID("ep-exhausted")
	freshID := shared.MediaItemID("ep-fresh")
	siblings := []*domainlibrary.MediaItem{
		{ID: exhaustedID, Type: domainlibrary.Episode, ParentID: parentID},
		{ID: freshID, Type: domainlibrary.Episode, ParentID: parentID},
	}
	segRepo := &fakeSegmentRepo{
		attempts: map[string]fakeIntroAttempt{
			string(exhaustedID): {count: maxIntroDetectionAttempts, version: introDetectionAlgoVersion},
		},
	}
	// GetByID returns this single item for both candidates: since the exhausted
	// one must never reach detectIntroWithHint (which dereferences item.File),
	// giving it a real file here would silently pass even with a bug - a nil
	// item.File would panic instead of just failing the assertion, which is a
	// stronger guard.
	dur, _ := shared.NewDurationMs(1_200_000)
	item := &domainlibrary.MediaItem{
		File: &domainlibrary.MediaFile{Path: "/tv/ep.mkv", Duration: dur},
	}
	itemRepo := &fakeMediaItemRepo{siblings: siblings, item: item}
	svc := newTestServiceFull(&fakeDetector{}, segRepo, itemRepo, &fakeTrickplayRepo{})

	svc.detectSeason(context.Background(), parentID)

	assert.Equal(t, 1, segRepo.exhaustedBatchCalls,
		"detectSeason must issue exactly one batched exhausted check per page, not one per candidate episode")

	freshAttempt, freshRan := segRepo.attempts[string(freshID)]
	assert.True(t, freshRan, "the non-exhausted episode must have gone through detection and recorded an attempt")
	assert.Equal(t, 1, freshAttempt.count)

	exhaustedAttempt := segRepo.attempts[string(exhaustedID)]
	assert.Equal(t, maxIntroDetectionAttempts, exhaustedAttempt.count,
		"the exhausted episode must be skipped entirely: no extra detection run, no extra recorded attempt")
}

// TestDetectSeason_ExhaustedBatchErrorFailsOpen proves that when the batched
// exhausted check itself errors, detectSeason does NOT skip anyone - matching
// the previous per-episode fail-open semantics (an error meant "don't skip").
func TestDetectSeason_ExhaustedBatchErrorFailsOpen(t *testing.T) {
	t.Parallel()

	parentID := "season-batch-err"
	epID := shared.MediaItemID("ep-batch-err")
	siblings := []*domainlibrary.MediaItem{
		{ID: epID, Type: domainlibrary.Episode, ParentID: parentID},
	}
	segRepo := &fakeSegmentRepo{
		attempts: map[string]fakeIntroAttempt{
			string(epID): {count: maxIntroDetectionAttempts, version: introDetectionAlgoVersion},
		},
		exhaustedBatchErr: errors.New("db unavailable"),
	}
	dur, _ := shared.NewDurationMs(1_200_000)
	item := &domainlibrary.MediaItem{
		File: &domainlibrary.MediaFile{Path: "/tv/ep.mkv", Duration: dur},
	}
	itemRepo := &fakeMediaItemRepo{siblings: siblings, item: item}
	svc := newTestServiceFull(&fakeDetector{}, segRepo, itemRepo, &fakeTrickplayRepo{})

	svc.detectSeason(context.Background(), parentID)

	assert.Equal(t, 1, segRepo.exhaustedBatchCalls)
	assert.Equal(t, maxIntroDetectionAttempts+1, segRepo.attempts[string(epID)].count,
		"a batch-check error must fail open: the episode still runs detection despite being exhausted per the (unreadable) attempts row")
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

// TestPropagateSegment_PaginatesAllSiblings: propagation must reach EVERY episode
// in the season, not just the first 100. A long single-season (anime, daily
// soaps) with >100 episodes would otherwise leave the later episodes without the
// propagated segment. We seed 250 siblings and assert every one was covered by
// the batched manual-lock read.
func TestPropagateSegment_PaginatesAllSiblings(t *testing.T) {
	t.Parallel()
	const total = 250 // > the 200 page size and the old 100 cap
	dur, _ := shared.NewDurationMs(3_600_000)
	ref := &domainlibrary.MediaItem{
		ID:       shared.MediaItemID("ep-ref"),
		Type:     domainlibrary.Episode,
		ParentID: "season-1",
		File:     &domainlibrary.MediaFile{Path: "/ep-ref.mkv", Duration: dur},
	}
	siblings := make([]*domainlibrary.MediaItem, total)
	for i := range total {
		siblings[i] = &domainlibrary.MediaItem{
			ID:       shared.MediaItemID(fmt.Sprintf("ep-%d", i)),
			Type:     domainlibrary.Episode,
			ParentID: "season-1",
			File:     &domainlibrary.MediaFile{Path: "/ep.mkv"},
		}
	}
	// Reference intro segment to propagate; frames at 0/15000/30000 ms (Interval=15,
	// 3 tile hashes) fall inside [0, 60000) so the reference hash set is non-empty.
	segRepo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{ID: "ref-seg", MediaItemID: ref.ID, Type: "intro", StartMs: 0, EndMs: 60_000, Source: "manual"},
	}}
	manifest := &domainlibrary.TrickplayManifest{
		MediaItemID: ref.ID, Interval: 15, TileWidth: 5, TileHeight: 3, Width: 160, Height: 90, ImageCount: 1,
	}
	det := &fakeDetector{tileHashes: []uint64{0x1111, 0x2222, 0x3333}}
	cacheDir := t.TempDir()
	// Only the reference needs a sidecar so its hash set is non-empty; siblings
	// fail extraction and are skipped - but each is still queried (manual-lock
	// check) before that, which is what proves the pagination coverage.
	writeTrickplaySidecarFixture(t, cacheDir, "ep-ref", 15, 15, 15)
	itemRepo := &fakeMediaItemRepo{item: ref, siblings: siblings}
	svc := NewIntroDetectionService(
		segRepo,
		&fakeTrickplayRepoWithManifest{manifest: manifest},
		itemRepo,
		&fakeConfigRepo{},
		det,
		cacheDir,
	)

	_, err := svc.PropagateSegment(context.Background(), "ep-ref", "intro")
	require.NoError(t, err)

	// Every sibling is covered by the batched manual-lock read, so
	// batchQueriedIDs proves the pagination reached every episode (past the old
	// 100 cap).
	queried := make(map[string]bool, len(segRepo.batchQueriedIDs))
	for _, id := range segRepo.batchQueriedIDs {
		queried[id] = true
	}
	require.True(t, queried["ep-249"], "propagation must page past 100 to reach the last episode")
	count := 0
	for i := range total {
		if queried[fmt.Sprintf("ep-%d", i)] {
			count++
		}
	}
	require.Equal(t, total, count, "every sibling must be visited by propagation")
}

// Propagation never overwrites a segment the admin set by hand on a sibling:
// the free sibling receives the propagated intro, the locked one keeps its
// manual segment untouched. The lock had no test (mutation 2026-09-28).
func TestPropagateSegment_SkipsASiblingWithAManualSegment(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3_600_000)
	ep := func(id string) *domainlibrary.MediaItem {
		return &domainlibrary.MediaItem{ID: shared.MediaItemID(id), Type: domainlibrary.Episode, ParentID: "season-1",
			File: &domainlibrary.MediaFile{Path: "/" + id + ".mkv", Duration: dur}}
	}
	ref, locked, free := ep("ep-ref"), ep("ep-locked"), ep("ep-free")
	segRepo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{ID: "ref-seg", MediaItemID: ref.ID, Type: "intro", StartMs: 0, EndMs: 40_000, Source: "manual", Confidence: 1.0},
		{ID: "locked-seg", MediaItemID: locked.ID, Type: "intro", StartMs: 5_000, EndMs: 20_000, Source: "manual"},
	}}
	manifest := &domainlibrary.TrickplayManifest{
		MediaItemID: ref.ID, Interval: 10, TileWidth: 5, TileHeight: 1, Width: 160, Height: 90, ImageCount: 1,
	}
	cacheDir := t.TempDir()
	// The manifest names ep-ref, so every episode resolves to the same sidecar
	// and matches the reference (same fixture as the batch-error test below).
	writeTrickplaySidecarFixture(t, cacheDir, "ep-ref", 10, 5, 5)
	svc := NewIntroDetectionService(segRepo, &fakeTrickplayRepoWithManifest{manifest: manifest},
		&fakeMediaItemRepo{item: ref, siblings: []*domainlibrary.MediaItem{locked, free}},
		&fakeConfigRepo{}, &fakeDetector{tileHashes: []uint64{0x1111, 0x2222, 0x3333, 0x4444, 0x5555}}, cacheDir)

	_, err := svc.PropagateSegment(context.Background(), "ep-ref", "intro")
	require.NoError(t, err)

	wrote := map[shared.MediaItemID]bool{}
	for _, sg := range segRepo.upserted {
		wrote[sg.MediaItemID] = true
	}
	require.True(t, wrote[free.ID], "control: the free sibling must receive the propagated intro")
	require.False(t, wrote[locked.ID], "a sibling with a manual intro must be left alone")
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

// --- H5 fail-closed regression tests ---

// TestUpsertSegmentRespectingManual_UpsertError_Propagates proves that when the
// atomic manual-lock upsert (UpsertUnlessManual) errors, DetectIntro propagates
// the error rather than swallowing it. The read-then-write guard was replaced by
// a single conditional statement, so the lock check can no longer fail
// independently of the write - the write error is the only failure mode left.
func TestUpsertSegmentRespectingManual_UpsertError_Propagates(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3_600_000)
	item := &domainlibrary.MediaItem{
		ID:   shared.MediaItemID("item-guardread"),
		Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{Path: "/movie.mkv", Duration: dur},
	}
	segRepo := &fakeSegmentRepo{
		upsertErr: errors.New("db transient error"),
	}
	det := &fakeDetector{
		creditsStartMs: 3_480_000,
		creditsEndMs:   3_600_000,
		creditsFound:   true,
	}
	svc := newTestService(det, segRepo, item)

	err := svc.DetectIntro(context.Background(), "item-guardread")
	require.Error(t, err, "DetectIntro must propagate the error when the manual-lock upsert fails")
}

// TestPropagateSegment_SiblingGetSegError_SkipsUpsert proves that when the
// batched sibling-segments read (the manual-lock guard) fails for the page,
// every sibling on that page is skipped entirely (fail-closed) rather than
// falling open and risking a clobbered operator "manual" mark. The test
// seeds matching hash data so the sibling WOULD be upserted without the
// error injection, distinguishing the fix from other skip reasons (no
// hashes, no manifest).
func TestPropagateSegment_SiblingGetSegError_SkipsUpsert(t *testing.T) {
	t.Parallel()
	ref := &domainlibrary.MediaItem{
		ID:       shared.MediaItemID("ep-ref"),
		Type:     domainlibrary.Episode,
		ParentID: "season-1",
		File:     &domainlibrary.MediaFile{Path: "/ep-ref.mkv"},
	}
	sib := &domainlibrary.MediaItem{
		ID:       shared.MediaItemID("ep-sib"),
		Type:     domainlibrary.Episode,
		ParentID: "season-1",
		File:     &domainlibrary.MediaFile{Path: "/ep-sib.mkv"},
	}
	refSeg := &domainlibrary.MediaSegment{
		ID:          "seg-ref",
		MediaItemID: ref.ID,
		Type:        "intro",
		StartMs:     0,
		EndMs:       40_000,
		Source:      "manual",
		Confidence:  1.0,
	}
	segRepo := &fakeSegmentRepo{
		seeded: []*domainlibrary.MediaSegment{refSeg},
		// Ref's own segment lookup (a single-item GetByMediaItemID, outside the
		// sibling batch) must still succeed; only the batched sibling read fails.
		batchErr: errors.New("db transient"),
	}
	// manifest.MediaItemID = "ep-ref" so both ref and sib resolve to the same
	// sidecar path, giving both 5 matching hashes - i.e. without the error
	// injection the sibling WOULD be upserted.
	manifest := &domainlibrary.TrickplayManifest{
		MediaItemID: ref.ID,
		Interval:    10, TileWidth: 5, TileHeight: 1,
		Width: 160, Height: 90, ImageCount: 1,
	}
	det := &fakeDetector{tileHashes: []uint64{0x1111, 0x2222, 0x3333, 0x4444, 0x5555}}
	cacheDir := t.TempDir()
	// 5 frames at [0, 10 000, 20 000, 30 000, 40 000] ms.
	writeTrickplaySidecarFixture(t, cacheDir, "ep-ref", 10, 5, 5)
	svc := NewIntroDetectionService(
		segRepo,
		&fakeTrickplayRepoWithManifest{manifest: manifest},
		&fakeMediaItemRepo{item: ref, siblings: []*domainlibrary.MediaItem{sib}},
		&fakeConfigRepo{},
		det,
		cacheDir,
	)

	propagated, err := svc.PropagateSegment(context.Background(), "ep-ref", "intro")
	require.NoError(t, err)
	require.Empty(t, propagated, "a batched sibling-segments read error must skip the whole page, not just append nothing")
	for _, u := range segRepo.upserted {
		if u.MediaItemID == sib.ID {
			t.Fatalf("Upsert must not be called for a sibling on a page whose lock-check batch read errored; got %+v", u)
		}
	}
	require.Equal(t, 1, segRepo.batchCalls, "PropagateSegment must issue exactly one batched sibling-segments read")
}

// TestDetectSeason_KilledDecodeConsumesAnAttempt pins the rule that made the
// 2026-08-24 incident survive three reboots: a decode the kernel killed must
// consume an attempt.
//
// The sweep deliberately does NOT count ordinary failures, so a transient I/O
// error cannot park a detectable episode. A decode killed by a signal is the
// opposite case - it is the OOM killer, it reproduces on the same file, and the
// work list is rebuilt from the database, so an uncounted kill is handed back
// after every restart. In prod that produced 23 kills, zero error lines, and a
// host taken down twice.
func TestDetectSeason_KilledDecodeConsumesAnAttempt(t *testing.T) {
	t.Parallel()

	parentID := "season-killed"
	epID := shared.MediaItemID("ep-killed")
	dur, _ := shared.NewDurationMs(1_200_000)
	siblings := []*domainlibrary.MediaItem{
		{
			ID: epID, Type: domainlibrary.Episode, ParentID: parentID,
			File: &domainlibrary.MediaFile{Path: "/tv/ep.mkv", Duration: dur},
		},
	}
	segRepo := &fakeSegmentRepo{}
	itemRepo := &fakeMediaItemRepo{siblings: siblings}
	det := &fakeDetector{fallbackErr: fmt.Errorf("ffmpeg run: %w", domainlibrary.ErrDetectorKilled)}
	svc := newTestServiceFull(det, segRepo, itemRepo, &fakeTrickplayRepo{})

	svc.detectSeason(context.Background(), parentID)

	attempt, recorded := segRepo.attempts[string(epID)]
	require.True(t, recorded,
		"a killed decode must record an attempt: the work list survives a restart, so an uncounted kill comes back and retakes the host down")
	require.Equal(t, 1, attempt.count, "exactly one attempt per killed decode")
}

// TestDetectSeason_TransientFailureDoesNotConsumeAnAttempt is the other half of
// the rule: the fix must not turn every hiccup into a parked episode.
func TestDetectSeason_TransientFailureDoesNotConsumeAnAttempt(t *testing.T) {
	t.Parallel()

	parentID := "season-transient"
	epID := shared.MediaItemID("ep-transient")
	dur, _ := shared.NewDurationMs(1_200_000)
	siblings := []*domainlibrary.MediaItem{
		{
			ID: epID, Type: domainlibrary.Episode, ParentID: parentID,
			File: &domainlibrary.MediaFile{Path: "/tv/ep.mkv", Duration: dur},
		},
	}
	segRepo := &fakeSegmentRepo{}
	itemRepo := &fakeMediaItemRepo{siblings: siblings}
	det := &fakeDetector{fallbackErr: errors.New("read /media/ep.mkv: input/output error")}
	svc := newTestServiceFull(det, segRepo, itemRepo, &fakeTrickplayRepo{})

	svc.detectSeason(context.Background(), parentID)

	_, recorded := segRepo.attempts[string(epID)]
	require.False(t, recorded,
		"a transient failure must not consume an attempt, or one bad mount parks a whole library as undetectable")
}

// TestDetectSeason_BudgetDeadlineConsumesAnAttempt pins the rule the 2026-08-29
// prod loop was missing: an episode that exhausts its detection budget must
// consume an attempt. The deadline reproduces on the same file, so an
// uncounted one is handed back by every idle sweep - prod re-ran the same
// three seasons every 90 minutes for a day, 30 minutes each, finding nothing.
func TestDetectSeason_BudgetDeadlineConsumesAnAttempt(t *testing.T) {
	t.Parallel()

	parentID := "season-deadline"
	epID := shared.MediaItemID("ep-deadline")
	dur, _ := shared.NewDurationMs(1_200_000)
	siblings := []*domainlibrary.MediaItem{
		{
			ID: epID, Type: domainlibrary.Episode, ParentID: parentID,
			File: &domainlibrary.MediaFile{Path: "/tv/ep.mkv", Duration: dur},
		},
	}
	segRepo := &fakeSegmentRepo{}
	itemRepo := &fakeMediaItemRepo{siblings: siblings}
	det := &fakeDetector{fallbackErr: fmt.Errorf("silence pass: %w", context.DeadlineExceeded)}
	svc := newTestServiceFull(det, segRepo, itemRepo, &fakeTrickplayRepo{})

	svc.detectSeason(context.Background(), parentID)

	attempt, recorded := segRepo.attempts[string(epID)]
	require.True(t, recorded,
		"a spent detection budget must record an attempt, or the idle sweep re-runs the same episode forever")
	require.Equal(t, 1, attempt.count, "exactly one attempt per exhausted budget")
}

// A shutdown (context canceled, not a deadline) is the sweep being killed, not
// the episode being undetectable: it must not consume an attempt.
func TestDetectSeason_ShutdownCancelDoesNotConsumeAnAttempt(t *testing.T) {
	t.Parallel()

	parentID := "season-canceled"
	epID := shared.MediaItemID("ep-canceled")
	dur, _ := shared.NewDurationMs(1_200_000)
	siblings := []*domainlibrary.MediaItem{
		{
			ID: epID, Type: domainlibrary.Episode, ParentID: parentID,
			File: &domainlibrary.MediaFile{Path: "/tv/ep.mkv", Duration: dur},
		},
	}
	segRepo := &fakeSegmentRepo{}
	itemRepo := &fakeMediaItemRepo{siblings: siblings}
	det := &fakeDetector{fallbackErr: fmt.Errorf("silence pass: %w", context.Canceled)}
	svc := newTestServiceFull(det, segRepo, itemRepo, &fakeTrickplayRepo{})

	svc.detectSeason(context.Background(), parentID)

	_, recorded := segRepo.attempts[string(epID)]
	require.False(t, recorded, "a canceled sweep must not park the episode it was interrupted on")
}
