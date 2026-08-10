package library

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	libraryfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

// ctxHonoringSegmentRepo mirrors the postgres repo's contract on an expired
// context: every write fails with ctx.Err(). The plain fake ignores the
// context, which is exactly how "found segments lost at the deadline" stayed
// green in tests while prod logged `upsert segment intro: context deadline
// exceeded` (46 occurrences, 2026-08-07..10).
type ctxHonoringSegmentRepo struct {
	fakeSegmentRepo
}

func (r *ctxHonoringSegmentRepo) UpsertUnlessManual(ctx context.Context, seg *domainlibrary.MediaSegment) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return r.fakeSegmentRepo.UpsertUnlessManual(ctx, seg)
}

// A detection budget spent on slow scans (or waiting in line) must not void
// the results at the write: the upsert costs milliseconds where re-detection
// costs another 30-minute decode. Prod trace (2026-08-09 23:44:58Z): cluster
// hint found an intro, then provider chain AND upsert failed in the same
// second - the whole pipeline ran on an expired context and the found intro
// was thrown away.
func TestDetectIntro_PersistsResultsWhenTheDetectionBudgetIsSpent(t *testing.T) {
	t.Parallel()
	mediaID := shared.MediaItemID("ep-1")
	item := &domainlibrary.MediaItem{
		ID:   mediaID,
		Type: domainlibrary.Episode,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
	}
	chapRepo := libraryfakes.NewFakeChapterRepository()
	require.NoError(t, chapRepo.ReplaceBatch(context.Background(), mediaID, []*domainlibrary.Chapter{
		{Title: "Intro", StartTimeMs: 30_000, EndTimeMs: 60_000},
		{Title: "Episode", StartTimeMs: 60_000, EndTimeMs: 1_800_000},
	}))
	repo := &ctxHonoringSegmentRepo{}
	svc := NewIntroDetectionService(
		repo,
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithMaxParallelDetections(0)
	svc.WithChapterRepo(chapRepo)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err := svc.DetectIntro(ctx, string(mediaID))

	require.NoError(t, err, "an expired detection budget must not void the found segments")
	var intro *domainlibrary.MediaSegment
	for _, seg := range repo.upserted {
		if seg.Type == "intro" {
			intro = seg
		}
	}
	require.NotNil(t, intro, "the chapter intro found before the deadline must be stored")
	assert.Equal(t, int64(30_000), intro.StartMs)
	assert.Equal(t, int64(60_000), intro.EndMs)
}

// Waiting for a busy concurrency slot must not consume the per-episode
// detection budget: under limiter contention (several sweeps piled up) prod
// episodes burned their whole 30-minute allowance in line, then every pass
// soft-failed in lockstep (failures logged exactly 30:00 apart).
func TestDetectIntro_SlotWaitDoesNotEatTheDetectionBudget(t *testing.T) {
	t.Parallel()
	mediaID := shared.MediaItemID("ep-1")
	item := &domainlibrary.MediaItem{
		ID:   mediaID,
		Type: domainlibrary.Episode,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
	}
	chapRepo := libraryfakes.NewFakeChapterRepository()
	require.NoError(t, chapRepo.ReplaceBatch(context.Background(), mediaID, []*domainlibrary.Chapter{
		{Title: "Intro", StartTimeMs: 30_000, EndTimeMs: 60_000},
	}))
	repo := &ctxHonoringSegmentRepo{}
	svc := NewIntroDetectionService(
		repo,
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithMaxParallelDetections(1)
	svc.WithChapterRepo(chapRepo)
	svc.episodeBudget = 100 * time.Millisecond

	// Hold the only slot well past the episode budget, then release it.
	svc.concurrencyLimiter <- struct{}{}
	go func() {
		time.Sleep(400 * time.Millisecond)
		<-svc.concurrencyLimiter
	}()

	err := svc.DetectIntro(context.Background(), string(mediaID))

	require.NoError(t, err, "the budget must start when a slot is held, not when the wait begins")
	require.NotEmpty(t, repo.upserted, "detection must run with a fresh budget after the wait")
}

// A second admin trigger for a library whose sweep is still running must be
// rejected instead of stacking a concurrent duplicate sweep: prod logged four
// simultaneous "admin backfill: starting" (2026-08-09 18:02Z), and the piled-up
// sweeps starved the shared limiter until episodes timed out.
func TestBackfillLibrary_RejectsDuplicateTriggerWhileRunning(t *testing.T) {
	t.Parallel()
	repo := &seasonBlockingRepo{release: make(chan struct{})}
	repo.siblings = backfillSeasons(2)
	svc := newBackfillSvc(repo, 1)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, err := svc.BackfillLibrary(context.Background(), "lib-1")
		assert.NoError(t, err)
	}()
	require.Eventually(t, func() bool {
		return repo.inflight.Load() >= 1
	}, 2*time.Second, 5*time.Millisecond, "first sweep never started")

	var dupErr atomic.Value
	dupDone := make(chan struct{})
	go func() {
		defer close(dupDone)
		n, err := svc.BackfillLibrary(context.Background(), "lib-1")
		assert.Zero(t, n)
		if err != nil {
			dupErr.Store(err)
		}
	}()
	select {
	case <-dupDone:
	case <-time.After(2 * time.Second):
		t.Fatal("duplicate trigger must return promptly, not join the running sweep")
	}
	err, _ := dupErr.Load().(error)
	require.ErrorIs(t, err, ErrIntroBackfillAlreadyRunning)

	close(repo.release)
	<-firstDone

	// The guard must release with the sweep: a later re-trigger runs normally.
	_, err = svc.BackfillLibrary(context.Background(), "lib-1")
	assert.NoError(t, err)
}
