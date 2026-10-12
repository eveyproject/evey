package library

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// blockingMediaItemRepo blocks every GetByID until release is closed,
// letting the test observe how many DetectIntro calls hold a semaphore
// slot at once.
type blockingMediaItemRepo struct {
	domainlibrary.MediaItemQueryRepo
	release     chan struct{}
	inflight    atomic.Int32
	maxInflight atomic.Int32
}

func (r *blockingMediaItemRepo) GetByID(ctx context.Context, _ shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	cur := r.inflight.Add(1)
	for {
		old := r.maxInflight.Load()
		if cur <= old {
			break
		}
		if r.maxInflight.CompareAndSwap(old, cur) {
			break
		}
	}
	defer r.inflight.Add(-1)
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, context.Canceled // short-circuit DetectIntro after the blocking gate
}
func (r *blockingMediaItemRepo) List(_ context.Context, _ domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	return nil, nil
}

func TestDetectIntro_ConcurrencyCap(t *testing.T) {
	t.Parallel()
	const cap = 2
	const callers = 6
	repo := &blockingMediaItemRepo{release: make(chan struct{})}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		repo,
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithMaxParallelDetections(cap)

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.DetectIntro(context.Background(), "media-1")
		}()
	}
	// Wait until inflight actually reaches the cap instead of guessing with a
	// fixed sleep: under -race on a loaded runner a goroutine may not reach the
	// blocking GetByID within a fixed window, which made the exact-peak assertion
	// flaky. maxInflight is a high-water mark the limiter can never push past
	// `cap`, so once it reaches cap it is safe to read deterministically.
	assert.Eventually(t, func() bool {
		return repo.maxInflight.Load() >= int32(cap)
	}, 2*time.Second, 5*time.Millisecond, "inflight never reached cap %d", cap)
	peak := repo.maxInflight.Load()
	close(repo.release)
	wg.Wait()

	assert.Equal(t, int32(cap), peak, "concurrencyLimiter must cap inflight to %d (saw %d)", cap, peak)
}

func TestDetectIntro_ConcurrencyDisabled(t *testing.T) {
	t.Parallel()
	const callers = 5
	repo := &blockingMediaItemRepo{release: make(chan struct{})}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		repo,
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithMaxParallelDetections(0) // disable cap

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.DetectIntro(context.Background(), "media-1")
		}()
	}
	assert.Eventually(t, func() bool {
		return repo.maxInflight.Load() >= int32(callers)
	}, 2*time.Second, 5*time.Millisecond, "inflight never reached %d", callers)
	peak := repo.maxInflight.Load()
	close(repo.release)
	wg.Wait()

	assert.Equal(t, int32(callers), peak, "with cap disabled all callers must run in parallel (saw %d)", peak)
}

func TestDetectIntro_ConcurrencyCancelDuringWait(t *testing.T) {
	t.Parallel()
	repo := &blockingMediaItemRepo{release: make(chan struct{})}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		repo,
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithMaxParallelDetections(1)

	// First caller acquires the only slot.
	firstDone := make(chan struct{})
	go func() {
		_ = svc.DetectIntro(context.Background(), "media-A")
		close(firstDone)
	}()

	// Wait until the first caller has actually entered the pipeline.
	deadline := time.Now().Add(time.Second)
	for repo.inflight.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, int32(1), repo.inflight.Load(), "first caller must have entered the pipeline")

	// Second caller waits for the slot. Cancelling its context must return
	// promptly with ctx.Err() instead of hanging until the first caller
	// finishes.
	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- svc.DetectIntro(ctx, "media-B")
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-secondDone:
		assert.ErrorIs(t, err, context.Canceled, "cancelled wait must return ctx.Err()")
	case <-time.After(time.Second):
		t.Fatal("second caller did not return after ctx cancel")
	}

	close(repo.release)
	<-firstDone
}

// seasonBlockingRepo serves the library's season listing normally but blocks
// every episode listing (ParentID filter) until release closes, recording how
// many seasons are inside detectSeason at once.
type seasonBlockingRepo struct {
	fakeMediaItemRepo
	release     chan struct{}
	panicParent string
	inflight    atomic.Int32
	maxInflight atomic.Int32
}

func (r *seasonBlockingRepo) List(ctx context.Context, f domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	if f.ParentID == nil {
		return r.fakeMediaItemRepo.List(ctx, f)
	}
	if r.panicParent != "" && *f.ParentID == r.panicParent {
		panic("poisoned season")
	}
	cur := r.inflight.Add(1)
	for {
		old := r.maxInflight.Load()
		if cur <= old || r.maxInflight.CompareAndSwap(old, cur) {
			break
		}
	}
	defer r.inflight.Add(-1)
	select {
	case <-r.release:
	case <-ctx.Done():
	}
	return nil, nil
}

func newBackfillSvc(repo *seasonBlockingRepo, maxParallel int) *IntroDetectionService {
	return NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		repo,
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithMaxParallelDetections(maxParallel)
}

func backfillSeasons(n int) []*domainlibrary.MediaItem {
	seasons := make([]*domainlibrary.MediaItem, n)
	for i := range seasons {
		seasons[i] = &domainlibrary.MediaItem{
			ID:   shared.MediaItemID(string(rune('a'+i)) + "-season"),
			Type: domainlibrary.Season,
		}
	}
	return seasons
}

func TestBackfillLibrary_ParallelSeasons(t *testing.T) {
	t.Parallel()
	const workers = 2
	repo := &seasonBlockingRepo{release: make(chan struct{})}
	repo.siblings = backfillSeasons(4)
	svc := newBackfillSvc(repo, workers)

	done := make(chan struct{})
	go func() {
		defer close(done)
		n, err := svc.BackfillLibrary(context.Background(), "lib-1")
		assert.NoError(t, err)
		assert.Equal(t, 4, n)
	}()

	assert.Eventually(t, func() bool {
		return repo.maxInflight.Load() >= workers
	}, 2*time.Second, 5*time.Millisecond, "backfill never ran %d seasons in parallel", workers)
	close(repo.release)
	<-done

	assert.Equal(t, int32(workers), repo.maxInflight.Load(),
		"backfill must fan out to exactly the pipeline budget (%d)", workers)
}

func TestBackfillLibrary_SequentialWhenBudgetIsOne(t *testing.T) {
	t.Parallel()
	repo := &seasonBlockingRepo{release: make(chan struct{})}
	repo.siblings = backfillSeasons(3)
	svc := newBackfillSvc(repo, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.BackfillLibrary(context.Background(), "lib-1")
	}()

	assert.Eventually(t, func() bool {
		return repo.maxInflight.Load() >= 1
	}, 2*time.Second, 5*time.Millisecond, "backfill never started")
	assert.Never(t, func() bool {
		return repo.maxInflight.Load() > 1
	}, 300*time.Millisecond, 10*time.Millisecond, "budget 1 must keep the sweep sequential")
	close(repo.release)
	<-done
}

// clampingSeasonRepo mimics the postgres repo's DefaultListLimit: any List
// asking for more than 500 rows (or 0) is silently clamped to 500. The plain
// fake honours whatever Limit it is given, which is precisely how the
// one-shot season listing stayed green in tests while prod truncated a
// 500+-season library (admin backfill logged seasons=500 twice, 2026-08-08).
type clampingSeasonRepo struct {
	fakeMediaItemRepo
	mu             sync.Mutex
	visitedParents map[string]struct{}
}

func (r *clampingSeasonRepo) List(ctx context.Context, f domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	if f.ParentID != nil {
		r.mu.Lock()
		if r.visitedParents == nil {
			r.visitedParents = map[string]struct{}{}
		}
		r.visitedParents[*f.ParentID] = struct{}{}
		r.mu.Unlock()
		return nil, nil // no episodes: detectSeason no-ops, only the visit counts
	}
	const repoCap = 500
	if f.Limit <= 0 || f.Limit > repoCap {
		f.Limit = repoCap
	}
	return r.fakeMediaItemRepo.List(ctx, f)
}

// The admin backfill must reach EVERY season of the library, not just the
// first repo page: a truncated sweep reports success while silently never
// detecting anything past the cap.
func TestBackfillLibrary_SweepsPastTheRepoListCap(t *testing.T) {
	t.Parallel()
	repo := &clampingSeasonRepo{}
	nSeasons := 502
	seasons := make([]*domainlibrary.MediaItem, nSeasons)
	for i := range seasons {
		seasons[i] = &domainlibrary.MediaItem{
			ID:   shared.MediaItemID(fmt.Sprintf("season-%03d", i)),
			Type: domainlibrary.Season,
		}
	}
	repo.siblings = seasons
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		repo,
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithMaxParallelDetections(2)

	n, err := svc.BackfillLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, nSeasons, n, "every season must be scheduled, not just the repo's first page")
	assert.Len(t, repo.visitedParents, nSeasons, "every season must actually enter detection")
}

func TestBackfillLibrary_PanicInOneSeasonDoesNotKillTheSweep(t *testing.T) {
	t.Parallel()
	repo := &seasonBlockingRepo{release: make(chan struct{}), panicParent: "a-season"}
	close(repo.release)
	repo.siblings = backfillSeasons(3)
	svc := newBackfillSvc(repo, 2)

	n, err := svc.BackfillLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 3, n, "the poisoned season must not abort the other seasons")
}
