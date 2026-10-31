package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/search"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSearchIndex struct {
	searchFunc     func(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error)
	indexFunc      func(ctx context.Context, item *library.MediaItem) error
	indexBatchFunc func(ctx context.Context, items []*library.MediaItem) error
	removeFunc     func(ctx context.Context, id string) error
	clearFunc      func(ctx context.Context) error
}

func (f *fakeSearchIndex) Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	return f.searchFunc(ctx, query, filters, sort, limit)
}

func (f *fakeSearchIndex) IndexMediaItem(ctx context.Context, item *library.MediaItem) error {
	if f.indexFunc != nil {
		return f.indexFunc(ctx, item)
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

func (f *fakeSearchIndex) Clear(ctx context.Context) error {
	if f.clearFunc != nil {
		return f.clearFunc(ctx)
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

// --- IndexMediaItem ---

func TestIndexMediaItem_NilPrimary_Noop(t *testing.T) {
	rs := search.NewResilient(nil, nil)
	err := rs.IndexMediaItem(context.Background(), &library.MediaItem{ID: "1"})
	require.NoError(t, err)
}

func TestIndexMediaItem_PrimarySuccess(t *testing.T) {
	var indexed bool
	primary := &fakeSearchIndex{
		indexFunc: func(_ context.Context, _ *library.MediaItem) error {
			indexed = true
			return nil
		},
	}
	rs := search.NewResilient(primary, nil)
	err := rs.IndexMediaItem(context.Background(), &library.MediaItem{ID: "1", CastNames: []string{"Some Actor"}})
	require.NoError(t, err)
	assert.True(t, indexed)
}

func TestIndexMediaItem_PrimaryFails_NoError(t *testing.T) {
	primary := &fakeSearchIndex{
		indexFunc: func(_ context.Context, _ *library.MediaItem) error {
			return errors.New("meili down")
		},
	}
	rs := search.NewResilient(primary, nil)
	err := rs.IndexMediaItem(context.Background(), &library.MediaItem{ID: "1"})
	require.NoError(t, err)
}

// --- IndexMediaItemsBatch ---

func TestIndexMediaItemsBatch_NilPrimary_Noop(t *testing.T) {
	rs := search.NewResilient(nil, nil)
	err := rs.IndexMediaItemsBatch(context.Background(), []*library.MediaItem{{ID: "1"}})
	require.NoError(t, err)
}

func TestIndexMediaItemsBatch_PrimarySuccess(t *testing.T) {
	var batchSize int
	primary := &fakeSearchIndex{
		indexBatchFunc: func(_ context.Context, items []*library.MediaItem) error {
			batchSize = len(items)
			return nil
		},
	}
	rs := search.NewResilient(primary, nil)
	err := rs.IndexMediaItemsBatch(context.Background(), []*library.MediaItem{{ID: "1"}, {ID: "2"}})
	require.NoError(t, err)
	assert.Equal(t, 2, batchSize)
}

func TestIndexMediaItemsBatch_PrimaryFails_NoError(t *testing.T) {
	primary := &fakeSearchIndex{
		indexBatchFunc: func(_ context.Context, _ []*library.MediaItem) error {
			return errors.New("meili down")
		},
	}
	rs := search.NewResilient(primary, nil)
	err := rs.IndexMediaItemsBatch(context.Background(), []*library.MediaItem{{ID: "1"}})
	require.NoError(t, err)
}

// --- RemoveMediaItem ---

func TestRemoveMediaItem_NilPrimary_Noop(t *testing.T) {
	rs := search.NewResilient(nil, nil)
	err := rs.RemoveMediaItem(context.Background(), "1")
	require.NoError(t, err)
}

func TestRemoveMediaItem_PrimarySuccess(t *testing.T) {
	var removedID string
	primary := &fakeSearchIndex{
		removeFunc: func(_ context.Context, id string) error {
			removedID = id
			return nil
		},
	}
	rs := search.NewResilient(primary, nil)
	err := rs.RemoveMediaItem(context.Background(), "item-42")
	require.NoError(t, err)
	assert.Equal(t, "item-42", removedID)
}

func TestRemoveMediaItem_PrimaryFails_NoError(t *testing.T) {
	primary := &fakeSearchIndex{
		removeFunc: func(_ context.Context, _ string) error {
			return errors.New("meili down")
		},
	}
	rs := search.NewResilient(primary, nil)
	err := rs.RemoveMediaItem(context.Background(), "1")
	require.NoError(t, err)
}

// --- Clear ---

func TestClear_NilPrimary_Noop(t *testing.T) {
	rs := search.NewResilient(nil, nil)
	require.NoError(t, rs.Clear(context.Background()))
}

func TestClear_DelegatesToPrimary(t *testing.T) {
	var cleared bool
	primary := &fakeSearchIndex{clearFunc: func(_ context.Context) error { cleared = true; return nil }}
	rs := search.NewResilient(primary, nil)
	require.NoError(t, rs.Clear(context.Background()))
	assert.True(t, cleared)
}

// Unlike Index/Remove (which swallow), Clear SURFACES the primary's error so the
// reconcile does not proceed to reindex - and log success - over a failed wipe,
// which would leave the stale docs in place.
func TestClear_SurfacesPrimaryError(t *testing.T) {
	primary := &fakeSearchIndex{clearFunc: func(_ context.Context) error { return errors.New("meili down") }}
	rs := search.NewResilient(primary, nil)
	err := rs.Clear(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "meili down")
}

// --- Primary accessor ---

func TestPrimary_ReturnsSearchIndex(t *testing.T) {
	primary := &fakeSearchIndex{}
	rs := search.NewResilient(primary, nil)
	assert.Equal(t, primary, rs.Primary())
}

func TestPrimary_NilReturnsNil(t *testing.T) {
	rs := search.NewResilient(nil, nil)
	assert.Nil(t, rs.Primary())
}

// --- Search with filters and sort ---

func TestSearch_WithFiltersAndSort(t *testing.T) {
	primary := &fakeSearchIndex{
		searchFunc: func(_ context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
			assert.Equal(t, "test", query)
			assert.Equal(t, "movie", filters["type"])
			assert.Equal(t, []string{"title:asc"}, sort)
			assert.Equal(t, int64(25), limit)
			return []library.SearchHit{{ID: "hit-1"}}, 1, nil
		},
	}
	rs := search.NewResilient(primary, nil)
	hits, total, err := rs.Search(context.Background(), "test", map[string]string{"type": "movie"}, []string{"title:asc"}, 25)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "hit-1", hits[0].ID)
}

type libraryRemover struct {
	removed []string
}

func (l *libraryRemover) RemoveLibrary(_ context.Context, id string) error {
	l.removed = append(l.removed, id)
	return nil
}

func TestResilient_RemoveLibraryReachesThePrimary(t *testing.T) {
	rm := &libraryRemover{}
	primary := struct {
		library.SearchIndex
		*libraryRemover
	}{&fakeSearchIndex{}, rm}
	r := search.NewResilient(primary, &fakeSearchIndex{})
	if err := r.RemoveLibrary(context.Background(), "lib-1"); err != nil {
		t.Fatal(err)
	}
	if len(rm.removed) != 1 || rm.removed[0] != "lib-1" {
		t.Fatalf("primary purged %v, want [lib-1]", rm.removed)
	}
}

type recordingQueue struct{ ops []string }

func (q *recordingQueue) Enqueue(_ context.Context, id, op string) error {
	q.ops = append(q.ops, op+":"+id)
	return nil
}
func (q *recordingQueue) List(context.Context, int) ([]library.PendingSearchWrite, error) {
	return nil, nil
}
func (q *recordingQueue) Delete(context.Context, library.PendingSearchWrite) error { return nil }

// With a submit-only writer, the request and scan paths never wait on the
// engine's task (that wait is up to minutes under load): writes go to the
// writer, and every upsert is queued so the drain confirms it on primary.
func TestResilient_SubmitOnlyWriterQueuesEveryUpsertForTheDrain(t *testing.T) {
	var primaryWrites, writerWrites int
	primary := &fakeSearchIndex{
		indexFunc:      func(context.Context, *library.MediaItem) error { primaryWrites++; return nil },
		indexBatchFunc: func(context.Context, []*library.MediaItem) error { primaryWrites++; return nil },
	}
	rm := &libraryRemover{}
	writer := struct {
		*fakeSearchIndex
		*libraryRemover
	}{&fakeSearchIndex{
		indexFunc:      func(context.Context, *library.MediaItem) error { writerWrites++; return nil },
		indexBatchFunc: func(context.Context, []*library.MediaItem) error { writerWrites++; return nil },
	}, rm}
	q := &recordingQueue{}
	r := search.NewResilient(primary, &fakeSearchIndex{}).WithQueue(q).WithSubmitOnlyWriter(writer)
	ctx := context.Background()

	require.NoError(t, r.IndexMediaItem(ctx, &library.MediaItem{ID: "a"}))
	require.NoError(t, r.IndexMediaItemsBatch(ctx, []*library.MediaItem{{ID: "b"}, {ID: "c"}}))
	require.NoError(t, r.RemoveLibrary(ctx, "lib-1"))

	assert.Equal(t, 0, primaryWrites, "the waiting adapter is the drain's, not the request's")
	assert.Equal(t, 2, writerWrites)
	assert.Equal(t, []string{"lib-1"}, rm.removed)
	assert.Equal(t, []string{"upsert:a", "upsert:b", "upsert:c"}, q.ops, "every submitted upsert is confirmed by the drain")
}

// Without the outbox a submit-only write could be lost: the writer is ignored.
func TestResilient_SubmitOnlyWriterNeedsTheQueue(t *testing.T) {
	var primaryWrites int
	primary := &fakeSearchIndex{indexFunc: func(context.Context, *library.MediaItem) error { primaryWrites++; return nil }}
	writer := &fakeSearchIndex{indexFunc: func(context.Context, *library.MediaItem) error {
		t.Fatal("submit-only writer used without an outbox")
		return nil
	}}
	r := search.NewResilient(primary, &fakeSearchIndex{}).WithSubmitOnlyWriter(writer)
	require.NoError(t, r.IndexMediaItem(context.Background(), &library.MediaItem{ID: "a"}))
	assert.Equal(t, 1, primaryWrites)
}
