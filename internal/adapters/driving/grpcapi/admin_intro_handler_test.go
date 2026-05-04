package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
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
