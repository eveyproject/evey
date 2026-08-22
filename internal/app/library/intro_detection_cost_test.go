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

// The silence fallback always answers with confidence 0.5 while the serving
// gate defaults to 0.55: its result is computed by a 600s blackdetect+
// silencedetect decode and then withheld from every player. Paying for a scan
// whose answer cannot be served under the current config is pure waste.
func TestRunPipeline_SkipsSilenceFallbackWhenItsAnswerCannotBeServed(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{}
	svc := costTestService(repo, det, ep, &fakeConfigRepo{})

	svc.detectSeason(context.Background(), "season-id")

	assert.Zero(t, int(det.fallbackCalls.Load()),
		"the default serve gate (0.55) withholds every 0.5 result, so the scan must not run")
}

// ... but the gate is admin-tunable and 0 disables it. The condition has to be
// re-read from the effective config, never baked in: an operator who lowered
// the threshold still gets the fallback they paid for.
func TestRunPipeline_RunsSilenceFallbackWhenTheGateAdmitsIt(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{}
	cfg := &fakeConfigRepo{values: map[string]string{
		IntroDetectionConfigKey: `{"min_serve_confidence": 0}`,
	}}
	svc := costTestService(repo, det, ep, cfg)

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, int(det.fallbackCalls.Load()),
		"with the gate disabled the fallback result is servable, so the scan must run")
}
