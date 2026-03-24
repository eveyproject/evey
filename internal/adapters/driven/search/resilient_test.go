package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/search"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSearchIndex struct {
	searchFunc          func(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error)
	indexFunc           func(ctx context.Context, item *library.MediaItem, castNames []string) error
	indexBatchFunc      func(ctx context.Context, items []*library.MediaItem) error
	removeFunc          func(ctx context.Context, id string) error
}

func (f *fakeSearchIndex) Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	return f.searchFunc(ctx, query, filters, sort, limit)
}

func (f *fakeSearchIndex) IndexMediaItem(ctx context.Context, item *library.MediaItem, castNames []string) error {
	if f.indexFunc != nil {
		return f.indexFunc(ctx, item, castNames)
	}
	return nil
}

func (f *fakeSearchIndex) IndexMediaItemsBatch(ctx context.Context, items []*library.MediaItem) error {
	if f.indexBatchFunc != nil {
		return f.indexBatchFunc(ctx, items)
	}
	return nil
}

func (f *fakeSearchIndex) RemoveMediaItem(ctx context.Context, id string) error {
	if f.removeFunc != nil {
		return f.removeFunc(ctx, id)
	}
	return nil
}

func TestSearch_PrimaryOK_ReturnsPrimaryResult(t *testing.T) {
	primary := &fakeSearchIndex{
		searchFunc: func(_ context.Context, _ string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
			return []library.SearchHit{{ID: "hit-1", Title: "Primary"}}, 1, nil
		},
	}
	fallback := &fakeSearchIndex{
		searchFunc: func(_ context.Context, _ string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
			t.Fatal("fallback should not be called")
			return nil, 0, nil
		},
	}

	rs := search.NewResilient(primary, fallback)
	hits, total, err := rs.Search(context.Background(), "test", nil, nil, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "hit-1", hits[0].ID)
}

func TestSearch_PrimaryFails_FallbackCalled(t *testing.T) {
	primary := &fakeSearchIndex{
		searchFunc: func(_ context.Context, _ string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
			return nil, 0, errors.New("meili down")
		},
	}
	fallback := &fakeSearchIndex{
		searchFunc: func(_ context.Context, _ string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
			return []library.SearchHit{{ID: "sql-1", Title: "Fallback"}}, 1, nil
		},
	}

	rs := search.NewResilient(primary, fallback)
	hits, total, err := rs.Search(context.Background(), "test", nil, nil, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "sql-1", hits[0].ID)
}

func TestSearch_BothFail_ReturnsError(t *testing.T) {
	primary := &fakeSearchIndex{
		searchFunc: func(_ context.Context, _ string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
			return nil, 0, errors.New("meili down")
		},
	}
	fallback := &fakeSearchIndex{
		searchFunc: func(_ context.Context, _ string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
			return nil, 0, errors.New("sql broken")
		},
	}

	rs := search.NewResilient(primary, fallback)
	_, _, err := rs.Search(context.Background(), "test", nil, nil, 10)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sql broken")
}

func TestSearch_NilPrimary_UsesFallback(t *testing.T) {
	fallback := &fakeSearchIndex{
		searchFunc: func(_ context.Context, _ string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
			return []library.SearchHit{{ID: "fb-1"}}, 1, nil
		},
	}

	rs := search.NewResilient(nil, fallback)
	hits, total, err := rs.Search(context.Background(), "q", nil, nil, 5)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "fb-1", hits[0].ID)
}
