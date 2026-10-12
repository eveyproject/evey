package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeIntroCacheInvalidator struct {
	calls      int
	lastTmdbID int
	lastSeason int
	lastEp     int
	rows       int64
	err        error
}

func (f *fakeIntroCacheInvalidator) Invalidate(_ context.Context, tmdbID, season, ep int) (int64, error) {
	f.calls++
	f.lastTmdbID = tmdbID
	f.lastSeason = season
	f.lastEp = ep
	return f.rows, f.err
}

func TestInvalidateIntroCache_OK(t *testing.T) {
	fake := &fakeIntroCacheInvalidator{rows: 3}
	h := &adminIntroHandler{cache: fake}

	resp, err := h.InvalidateIntroCache(context.Background(), &eveyv1.InvalidateIntroCacheRequest{
		TmdbId:  1396,
		Season:  1,
		Episode: 5,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(3), resp.RowsDeleted)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, 1396, fake.lastTmdbID)
	assert.Equal(t, 1, fake.lastSeason)
	assert.Equal(t, 5, fake.lastEp)
}

func TestInvalidateIntroCache_ZeroTmdbID_ReturnsInvalidArgument(t *testing.T) {
	h := &adminIntroHandler{cache: &fakeIntroCacheInvalidator{}}

	_, err := h.InvalidateIntroCache(context.Background(), &eveyv1.InvalidateIntroCacheRequest{
		TmdbId: 0,
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestInvalidateIntroCache_RepoError_ReturnsInternal(t *testing.T) {
	fake := &fakeIntroCacheInvalidator{err: errors.New("db down")}
	h := &adminIntroHandler{cache: fake}

	_, err := h.InvalidateIntroCache(context.Background(), &eveyv1.InvalidateIntroCacheRequest{
		TmdbId: 1396,
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// fakeIntroBackfiller records calls to BackfillLibrary.
type fakeIntroBackfiller struct {
	libraryIDsReceived []string
	result             int
	err                error
	done               chan struct{}
}

func newFakeIntroBackfiller() *fakeIntroBackfiller {
	return &fakeIntroBackfiller{done: make(chan struct{}, 10)}
}

func (f *fakeIntroBackfiller) BackfillLibrary(_ context.Context, libraryID string) (int, error) {
	f.libraryIDsReceived = append(f.libraryIDsReceived, libraryID)
	f.done <- struct{}{}
	return f.result, f.err
}

func TestBackfillIntroDetection_StartsJobAndReturnsOK(t *testing.T) {
	fake := newFakeIntroBackfiller()
	fake.result = 5
	h := &adminIntroHandler{backfill: fake}

	resp, err := h.BackfillIntroDetection(context.Background(), &eveyv1.BackfillIntroDetectionRequest{
		LibraryId: "lib-123",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Wait for the goroutine to call BackfillLibrary.
	select {
	case <-fake.done:
	case <-time.After(2 * time.Second):
		t.Fatal("background goroutine did not call BackfillLibrary within timeout")
	}
	assert.Contains(t, fake.libraryIDsReceived, "lib-123")
}

func TestBackfillIntroDetection_AlreadyRunning_ReturnsAlreadyExists(t *testing.T) {
	h := &adminIntroHandler{backfill: newFakeIntroBackfiller()}
	h.backfillRunning = true

	_, err := h.BackfillIntroDetection(context.Background(), &eveyv1.BackfillIntroDetectionRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestBackfillIntroDetection_NilBackfiller_ReturnsUnavailable(t *testing.T) {
	h := &adminIntroHandler{}

	_, err := h.BackfillIntroDetection(context.Background(), &eveyv1.BackfillIntroDetectionRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}
