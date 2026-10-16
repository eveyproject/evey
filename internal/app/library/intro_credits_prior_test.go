package library

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// creditsSeason: settled episodes (chapter intro, pipeline skipped) "ep-<i+1>"
// of 45+i minutes, each with blackdetect credits leadSec[i] before the end.
func creditsSeason(t *testing.T, leadSec []int64) (*fakeSegmentRepo, []*domainlibrary.MediaItem) {
	t.Helper()
	repo := &fakeSegmentRepo{}
	var eps []*domainlibrary.MediaItem
	for i, lead := range leadSec {
		idx := i + 1
		durMs := int64(45+i) * 60 * 1000
		id := shared.MediaItemID(fmt.Sprintf("ep-%d", idx))
		eps = append(eps, &domainlibrary.MediaItem{
			ID: id, Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: fmt.Sprintf("/tv/%s.mkv", id), Duration: mustDurationMs(t, durMs)},
		})
		repo.seeded = append(repo.seeded,
			&domainlibrary.MediaSegment{MediaItemID: id, Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "mkv_chapter", Confidence: 1.0},
			&domainlibrary.MediaSegment{MediaItemID: id, Type: "end_credits", StartMs: durMs - lead*1000, EndMs: durMs, Source: "blackdetect", Confidence: 0.6},
		)
	}
	return repo, eps
}

type withinCall struct {
	path       string
	maxLeadSec float64
}

// Raised by Wolves S01 as measured in prod (2026-10-15): the three fades are
// re-scanned held to median 62s + 60s floor = 122s, the pilot at 85s is kept.
func TestReconcileSeasonCredits_RescansLeadOutliersInsideTheSeasonsStretch(t *testing.T) {
	t.Parallel()
	repo, eps := creditsSeason(t, []int64{85, 62, 261, 242, 62, 62, 62, 62, 62, 212})
	var calls []withinCall
	detector := &fakeDetector{creditsWithin: func(path string, durationSec, maxLeadSec float64) (int64, int64, bool, error) {
		calls = append(calls, withinCall{path: path, maxLeadSec: maxLeadSec})
		durMs := int64(durationSec * 1000)
		return durMs - 62_000, durMs, true, nil
	}}
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, detector, "/tmp/trickplay")

	svc.detectSeason(context.Background(), "season-id")

	require.Len(t, calls, 3, "only the three outliers pay a decode")
	for _, c := range calls {
		assert.InDelta(t, 122.0, c.maxLeadSec, 0.001, "the re-scan is held to median + band, not the default window")
	}
	assert.ElementsMatch(t, []string{"/tv/ep-3.mkv", "/tv/ep-4.mkv", "/tv/ep-10.mkv"},
		[]string{calls[0].path, calls[1].path, calls[2].path})

	require.Len(t, repo.upserted, 3)
	for _, got := range repo.upserted {
		assert.Equal(t, "end_credits", got.Type)
		assert.Equal(t, "blackdetect", got.Source)
		assert.Equal(t, int64(62_000), got.EndMs-got.StartMs, "the row adopts the black found inside the stretch")
		assert.InDelta(t, 0.6, got.Confidence, 1e-9, "a measured row keeps the detector's confidence")
	}
	assert.Empty(t, repo.deleted)
}

// No black inside the stretch: withheld under the serve gate, not deleted.
func TestReconcileSeasonCredits_WithholdsAnOutlierWhoseStretchHasNoBlack(t *testing.T) {
	t.Parallel()
	repo, eps := creditsSeason(t, []int64{62, 62, 62, 62, 250})
	detector := &fakeDetector{} // creditsWithin nil: nothing found
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, detector, "/tmp/trickplay")

	svc.detectSeason(context.Background(), "season-id")

	require.Len(t, repo.upserted, 1)
	got := repo.upserted[0]
	assert.Equal(t, shared.MediaItemID("ep-5"), got.MediaItemID)
	assert.Equal(t, int64(250_000), got.EndMs-got.StartMs, "the start is kept: nothing was measured")
	assert.InDelta(t, creditsPriorWithheldConf, got.Confidence, 1e-9)
	assert.Empty(t, repo.deleted)
}

// A withheld row is judged: a season swept twice pays the decode once.
func TestReconcileSeasonCredits_IsIdempotentAcrossSweeps(t *testing.T) {
	t.Parallel()
	repo, eps := creditsSeason(t, []int64{62, 62, 62, 62, 250})
	calls := 0
	detector := &fakeDetector{creditsWithin: func(string, float64, float64) (int64, int64, bool, error) {
		calls++
		return 0, 0, false, nil
	}}
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, detector, "/tmp/trickplay")

	svc.detectSeason(context.Background(), "season-id")
	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, calls, "the withheld row is not re-scanned on the second sweep")
	assert.Len(t, repo.upserted, 1)
}

func TestReconcileSeasonCredits_NeedsFourVoters(t *testing.T) {
	t.Parallel()
	repo, eps := creditsSeason(t, []int64{62, 62, 250})
	calls := 0
	detector := &fakeDetector{creditsWithin: func(string, float64, float64) (int64, int64, bool, error) {
		calls++
		return 0, 0, false, nil
	}}
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, detector, "/tmp/trickplay")

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 0, calls)
	assert.Empty(t, repo.upserted)
}

// Scattered leads widen the MAD and the band: nothing reads as an outlier.
func TestReconcileSeasonCredits_StaysQuietOnAScatteredSeason(t *testing.T) {
	t.Parallel()
	repo, eps := creditsSeason(t, []int64{60, 100, 140, 180, 220, 260})
	calls := 0
	detector := &fakeDetector{creditsWithin: func(string, float64, float64) (int64, int64, bool, error) {
		calls++
		return 0, 0, false, nil
	}}
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, detector, "/tmp/trickplay")

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 0, calls)
	assert.Empty(t, repo.upserted)
}

func TestReconcileSeasonCredits_LeavesAuthoritativeRowsAlone(t *testing.T) {
	t.Parallel()
	repo, eps := creditsSeason(t, []int64{62, 62, 62, 62, 62, 62})
	for _, sg := range repo.seeded {
		if sg.Type != "end_credits" {
			continue
		}
		switch sg.MediaItemID {
		case "ep-5":
			sg.Source, sg.StartMs = "mkv_chapter", sg.EndMs-300_000
		case "ep-6":
			sg.Source, sg.StartMs = "manual", sg.EndMs-300_000
		}
	}
	calls := 0
	detector := &fakeDetector{creditsWithin: func(string, float64, float64) (int64, int64, bool, error) {
		calls++
		return 0, 0, false, nil
	}}
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, detector, "/tmp/trickplay")

	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 0, calls)
	assert.Empty(t, repo.upserted)
}

// Rows past the absolute bound are the suspect repair's: they must not
// stretch the band and hide a fade inside it.
func TestReconcileSeasonCredits_ImplausibleRowsDoNotVote(t *testing.T) {
	t.Parallel()
	repo, eps := creditsSeason(t, []int64{62, 62, 62, 62, 200, 540, 600})
	var paths []string
	detector := &fakeDetector{creditsWithin: func(path string, durationSec, _ float64) (int64, int64, bool, error) {
		paths = append(paths, path)
		durMs := int64(durationSec * 1000)
		return durMs - 62_000, durMs, true, nil
	}}
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, detector, "/tmp/trickplay")

	svc.detectSeason(context.Background(), "season-id")

	assert.Contains(t, paths, "/tv/ep-5.mkv", "the 200s fade inside the bound is the prior's")
	assert.NotContains(t, paths, "/tv/ep-6.mkv")
	assert.NotContains(t, paths, "/tv/ep-7.mkv")
}
