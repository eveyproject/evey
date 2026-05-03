package library

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// blockingMediaItemRepo blocks every GetByID until release is closed,
// letting the test observe how many DetectIntro calls hold a semaphore
// slot at once.
type blockingMediaItemRepo struct {
	release    chan struct{}
	inflight   int32
	maxInflight int32
}

func (r *blockingMediaItemRepo) GetByID(ctx context.Context, _ shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	cur := atomic.AddInt32(&r.inflight, 1)
	for {
		old := atomic.LoadInt32(&r.maxInflight)
		if cur <= old {
			break
		}
		if atomic.CompareAndSwapInt32(&r.maxInflight, old, cur) {
			break
		}
	}
	defer atomic.AddInt32(&r.inflight, -1)
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, context.Canceled // short-circuit DetectIntro after the blocking gate
}
func (r *blockingMediaItemRepo) Create(_ context.Context, _ *domainlibrary.MediaItem) error { return nil }
func (r *blockingMediaItemRepo) List(_ context.Context, _ domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	return nil, nil
}
func (r *blockingMediaItemRepo) Update(_ context.Context, _ *domainlibrary.MediaItem) error { return nil }
func (r *blockingMediaItemRepo) Delete(_ context.Context, _ shared.MediaItemID) error       { return nil }
func (r *blockingMediaItemRepo) GetByFilePath(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}
func (r *blockingMediaItemRepo) GetByFileFingerprint(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}
func (r *blockingMediaItemRepo) GetByLibraryTypeTitle(_ context.Context, _ shared.LibraryID, _, _, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}
func (r *blockingMediaItemRepo) Count(_ context.Context, _ domainlibrary.ListMediaItemsFilter) (int, error) {
	return 0, nil
}
func (r *blockingMediaItemRepo) SeriesAggregate(_ context.Context, _ shared.MediaItemID) (domainlibrary.SeriesAggregate, error) {
	return domainlibrary.SeriesAggregate{}, nil
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
	// Give the goroutines time to acquire / queue. 100ms is generous: the
	// blocking GetByID returns instantly once release is closed, so the
	// peak inflight count is captured before the unblock.
	time.Sleep(100 * time.Millisecond)
	peak := atomic.LoadInt32(&repo.maxInflight)
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
	time.Sleep(100 * time.Millisecond)
	peak := atomic.LoadInt32(&repo.maxInflight)
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
	for atomic.LoadInt32(&repo.inflight) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.inflight), "first caller must have entered the pipeline")

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
