package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// Cost cuts on the sweep's per-episode ffmpeg bill. Every test here counts
// detector calls: the passes are minutes of decode each, so "was it called"
// is the only measurement that matters at the unit level.

const costTestDurMs = 42 * 60 * 1000

// uncoveredEpisode builds an episode with NO intro segment, so the sweep enters
// the pipeline on it (the expensive path this file polices).
func uncoveredEpisode(t *testing.T, seeded ...*domainlibrary.MediaSegment) (*fakeSegmentRepo, *domainlibrary.MediaItem) {
	t.Helper()
	idx := 1
	ep := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("ep-1"), Type: domainlibrary.Episode, IndexNumber: &idx,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv", Duration: mustDurationMs(t, costTestDurMs)},
	}
	for _, sg := range seeded {
		sg.MediaItemID = ep.ID
	}
	return &fakeSegmentRepo{seeded: seeded}, ep
}

func costTestService(repo *fakeSegmentRepo, det IntroDetector, ep *domainlibrary.MediaItem, cfg *fakeConfigRepo) *IntroDetectionService {
	return NewIntroDetectionService(
		repo, &fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: ep, siblings: []*domainlibrary.MediaItem{ep}},
		cfg, det, "/tmp/trickplay",
	)
}

// The end-credits scan is a full blackdetect decode of the last 20% of the file
// - 40-50% of one episode's detection cost. An episode that already carries a
// PLAUSIBLE credits marker gets nothing from re-running the same detector over
// the same file, yet the sweep paid for it on every pass.
func TestDetectSeason_SkipsCreditsScanWhenStoredCreditsArePlausible(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t, &domainlibrary.MediaSegment{
		Type: "end_credits", StartMs: costTestDurMs - 90_000, EndMs: costTestDurMs,
		Source: "blackdetect", Confidence: 0.6,
	})
	det := &countingDetector{}
	svc := costTestService(repo, det, ep, &fakeConfigRepo{})

	svc.detectSeason(context.Background(), "season-id")

	assert.Zero(t, int(det.creditsCalls.Load()),
		"an episode already carrying a plausible credits marker must not re-pay the blackdetect scan")
}

// The cut must not swallow the episodes that DO need the scan: no stored
// credits row at all means the pass is the only way to get one.
func TestDetectSeason_RunsCreditsScanWhenNoCreditsStored(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{fakeDetector: fakeDetector{
		creditsFound: true, creditsStartMs: costTestDurMs - 80_000, creditsEndMs: costTestDurMs,
	}}
	svc := costTestService(repo, det, ep, &fakeConfigRepo{})

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, int(det.creditsCalls.Load()),
		"an episode with no credits marker must still get one scan")
	require.NotEmpty(t, repo.upserted)
}

// A SUSPECT stored row keeps the repair contract of refreshSuspectCredits: it
// is re-scanned and, when the re-scan is no better, the row is dropped. The
// cut must route through that single path, not invent a second answer.
func TestDetectSeason_SuspectCreditsOnUncoveredEpisodeStillRepairOrDrop(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t, &domainlibrary.MediaSegment{
		Type: "end_credits", StartMs: costTestDurMs - 9*60*1000, EndMs: costTestDurMs,
		Source: "blackdetect", Confidence: 0.6,
	})
	det := &countingDetector{fakeDetector: fakeDetector{creditsFound: false}}
	svc := costTestService(repo, det, ep, &fakeConfigRepo{})

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, int(det.creditsCalls.Load()), "a suspect row is worth exactly one repair scan")
	assert.Contains(t, repo.deleted, "ep-1/end_credits",
		"an unrepairable suspect row is dropped, as refreshSuspectCredits already decided")
}

// The silence fallback answers at a fixed 0.5 while the serving gate defaults
// to 0.55, so its result is stored and then withheld from every player. It is
// computed anyway: the sweep is already decoding this episode, and a value
// nobody serves today is still eval material for the algorithm. Recomputing it
// later means re-sweeping the whole catalogue, which costs far more than
// paying for it now (user decision 2026-08-22).
func TestRunPipeline_ComputesTheSilenceFallbackEvenWhenItsAnswerCannotBeServed(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{}
	svc := costTestService(repo, det, ep, &fakeConfigRepo{})

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, int(det.fallbackCalls.Load()),
		"the default config computes and stores the fallback, whatever the serve gate withholds")
}

// The saving stays available behind an explicit opt-in, for the day the sweep's
// speed matters more than the eval corpus.
func TestRunPipeline_SkipsUnservableSilenceFallbackWhenTheAdminOptsIn(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{}
	cfg := &fakeConfigRepo{values: map[string]string{
		IntroDetectionConfigKey: `{"skip_unservable_silence_fallback": true}`,
	}}
	svc := costTestService(repo, det, ep, cfg)

	svc.detectSeason(context.Background(), "season-id")

	assert.Zero(t, int(det.fallbackCalls.Load()),
		"opted in, the scan whose 0.5 answer the 0.55 gate withholds must not run")
}

// Opted in, the skip is still conditional on the gate: one decision point, and
// it re-reads the effective config. An operator who lowered the threshold gets
// the fallback back even with the opt-in on, because its answer is servable.
func TestRunPipeline_OptedInStillRunsTheFallbackWhenTheGateAdmitsIt(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{}
	cfg := &fakeConfigRepo{values: map[string]string{
		IntroDetectionConfigKey: `{"skip_unservable_silence_fallback": true, "min_serve_confidence": 0}`,
	}}
	svc := costTestService(repo, det, ep, cfg)

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, int(det.fallbackCalls.Load()),
		"with the gate disabled the fallback result is servable, so the scan must run")
}

// Storing is not serving. The row the pipeline now writes back carries the 0.5
// tier, and the serve gate must still withhold it under the default config -
// the eval corpus grows without a coin-flip skip button reaching a player.
func TestRunPipeline_StoresTheFallbackRowButTheServeGateStillWithholdsIt(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{fakeDetector: fakeDetector{
		fallbackFound: true, fallbackStartMs: 12_000, fallbackEndMs: 45_000,
	}}
	svc := costTestService(repo, det, ep, &fakeConfigRepo{})

	svc.detectSeason(context.Background(), "season-id")

	var stored *domainlibrary.MediaSegment
	for _, sg := range repo.upserted {
		if sg.Type == "intro" {
			stored = sg
		}
	}
	require.NotNil(t, stored, "the fallback result must be persisted")
	assert.Equal(t, "audio", stored.Source)
	assert.InDelta(t, silenceFallbackConfidence, stored.Confidence, 1e-9)

	served, err := segQuerySvc([]*domainlibrary.MediaSegment{stored}).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	assert.Empty(t, served, "a stored 0.5 row stays withheld from players under the default gate")
}
