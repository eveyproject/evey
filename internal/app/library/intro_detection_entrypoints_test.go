package library

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// The dry run is the admin's read-only measurement: it returns what the
// pipeline WOULD store and persists nothing, so an authoritative intro already
// on the episode is never clobbered while the detector is being judged.
func TestDetectIntroDryRun_ReturnsWithoutPersisting(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	det := &countingDetector{fallbackFound: true, fallbackStartMs: 12_000, fallbackEndMs: 45_000}
	svc := costTestService(repo, det, ep, &fakeConfigRepo{})

	segs, err := svc.DetectIntroDryRun(context.Background(), "ep-1")
	require.NoError(t, err)
	var intro *domainlibrary.MediaSegment
	for i := range segs {
		if segs[i].Type == "intro" {
			intro = &segs[i]
		}
	}
	require.NotNil(t, intro, "the fallback intro must be reported")
	assert.Equal(t, int64(12_000), intro.StartMs)
	assert.Equal(t, int64(45_000), intro.EndMs)
	assert.Empty(t, repo.upserted, "a dry run must not write a single segment")
	// The end-credits scan is skipped on purpose: it is a whole-file decode
	// the eval harness does not score.
	assert.Zero(t, int(det.creditsCalls.Load()), "dry run must not run the credits scan")
}

func TestDetectIntroDryRun_Refusals(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	noFile := &domainlibrary.MediaItem{ID: "nofile", Type: domainlibrary.Episode}
	// Siblings only: an unknown id is really unknown (the `item` shortcut
	// answers every id).
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: []*domainlibrary.MediaItem{ep, noFile}}, &fakeConfigRepo{}, &countingDetector{}, "/tmp/trickplay")
	if _, err := svc.DetectIntroDryRun(context.Background(), "ghost"); err == nil {
		t.Error("unknown media must be an error")
	}
	if _, err := svc.DetectIntroDryRun(context.Background(), "nofile"); err == nil {
		t.Error("an item without a file must be an error")
	}
}

// DetectSeries pages through the series' seasons and runs each one; a list
// failure is reported, not swallowed.
func TestDetectSeries(t *testing.T) {
	t.Parallel()
	repo, ep := uncoveredEpisode(t)
	ep.ParentID = "season-1"
	seasons := []*domainlibrary.MediaItem{
		{ID: "season-1", ParentID: "series-1", Type: domainlibrary.Season},
		{ID: "season-2", ParentID: "series-1", Type: domainlibrary.Season},
	}
	items := append([]*domainlibrary.MediaItem{ep}, seasons...)
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{item: ep, siblings: items}, &fakeConfigRepo{}, &countingDetector{}, "/tmp/trickplay")

	n, err := svc.DetectSeries(context.Background(), "series-1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	failing := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &listErrRepo{err: errors.New("db down")}, &fakeConfigRepo{}, &countingDetector{}, "/tmp/trickplay")
	if _, err := failing.DetectSeries(context.Background(), "series-1"); err == nil {
		t.Error("a failing season list must surface")
	}
	if err := svc.DetectSeason(context.Background(), string(shared.MediaItemID("season-1"))); err != nil {
		t.Errorf("DetectSeason never fails past scheduling: %v", err)
	}
}

// listErrRepo fails every List, for the paths that must surface it.
type listErrRepo struct {
	fakeMediaItemRepo
	err error
}

func (r *listErrRepo) List(context.Context, domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	return nil, r.err
}
