package library_test

import (
	"context"
	"errors"
	"testing"

	applibrary "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedItem(repo *libfakes.FakeMediaItemQueryRepo, id, libID string) {
	repo.Seed(&library.MediaItem{
		ID:        shared.MediaItemID(id),
		LibraryID: shared.LibraryID(libID),
		Title:     id,
	})
}

func TestReindexSearchIndex_AllLibraries(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	seedItem(repo, "m3", "libB")
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, nil, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, 3, search.Indexed(), "every item across libraries indexed")
}

// fakeTranslationReader returns canned per-item translations and can be made to
// fail, to exercise the reindex's translation-attach path.
type fakeTranslationReader struct {
	byID map[string][]*library.MediaItemTranslation
	err  error
}

func (f *fakeTranslationReader) GetByMediaItems(_ context.Context, ids []string) (map[string][]*library.MediaItemTranslation, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string][]*library.MediaItemTranslation, len(ids))
	for _, id := range ids {
		if tr, ok := f.byID[id]; ok {
			out[id] = tr
		}
	}
	return out, nil
}

func TestReindexSearchIndex_AttachesTranslations(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	search := libfakes.NewFakeSearchIndex()
	tr := &fakeTranslationReader{byID: map[string][]*library.MediaItemTranslation{
		"m1": {{MediaItemID: "m1", Locale: "fr-FR", Title: "Le Voyage"}},
	}}

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, tr, nil, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	m1, ok := search.Get("m1")
	require.True(t, ok)
	require.Len(t, m1.Translations, 1, "m1 must carry its translation into the index")
	assert.Equal(t, "Le Voyage", m1.Translations[0].Title)

	m2, ok := search.Get("m2")
	require.True(t, ok)
	assert.Empty(t, m2.Translations, "m2 has no translation → none attached")
}

// A translation-table read error must NOT abort the reindex (a recovery path):
// the page is indexed without translations.
func TestReindexSearchIndex_TranslationError_StillIndexes(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	search := libfakes.NewFakeSearchIndex()
	tr := &fakeTranslationReader{err: errors.New("translations table down")}

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, tr, nil, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, search.Indexed(), "reindex must still index the item despite the translation read error")
}

// fakeCastReader returns canned per-item cast names and can be made to fail,
// to exercise the reindex's cast-attach path.
type fakeCastReader struct {
	byID map[string][]string
	err  error
}

func (f *fakeCastReader) CastNamesByMediaItems(_ context.Context, ids []string) (map[string][]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string][]string, len(ids))
	for _, id := range ids {
		if names, ok := f.byID[id]; ok {
			out[id] = names
		}
	}
	return out, nil
}

// A batch rebuild used to push documents with an empty cast (ToDocument got a
// nil cast on the batch path), silently breaking actor search after every
// reindex/reconcile/drain until the next TMDB re-enrich. The reindex must
// attach the DB cast names to each page.
func TestReindexSearchIndex_AttachesCast(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	search := libfakes.NewFakeSearchIndex()
	cast := &fakeCastReader{byID: map[string][]string{
		"m1": {"Some Actor", "Other Actor"},
	}}

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, cast, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	m1, ok := search.Get("m1")
	require.True(t, ok)
	assert.Equal(t, []string{"Some Actor", "Other Actor"}, m1.CastNames, "m1 must carry its cast into the index")

	m2, ok := search.Get("m2")
	require.True(t, ok)
	assert.Empty(t, m2.CastNames, "m2 has no cast → none attached")
}

// Same posture as translations: a cast read error must NOT abort the reindex,
// the page is indexed without cast.
func TestReindexSearchIndex_CastError_StillIndexes(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	search := libfakes.NewFakeSearchIndex()
	cast := &fakeCastReader{err: errors.New("people table down")}

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, cast, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, search.Indexed(), "reindex must still index the item despite the cast read error")
}

// fakeDocCounter stands in for the search engine's DocumentCount, so the
// reconcile decision (skip / in-sync / drift-rebuild) is unit-tested without a
// live Meilisearch.
type fakeDocCounter struct {
	count int64
	err   error
}

func (f fakeDocCounter) DocumentCount(_ context.Context) (int64, error) { return f.count, f.err }

func TestReconcileSearchIndex_SkipsWhenEngineUnreachable(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReconcileSearchIndex(context.Background(), repo, search,
		fakeDocCounter{err: errors.New("connection refused")}, nil, nil)
	require.NoError(t, err, "an unreachable engine is a skip, not an error")
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, search.Indexed(), "must not scan/index against a down engine")
}

func TestReconcileSearchIndex_InSync_NoRebuild(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReconcileSearchIndex(context.Background(), repo, search,
		fakeDocCounter{count: 2}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, search.Indexed(), "doc count matches db count → no rebuild")
}

func TestReconcileSearchIndex_DriftRebuilds(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	seedItem(repo, "m3", "libB")
	search := libfakes.NewFakeSearchIndex()

	// Engine holds 1 doc, DB holds 3 - drift (writes dropped during an outage)
	// → full rebuild.
	n, err := applibrary.ReconcileSearchIndex(context.Background(), repo, search,
		fakeDocCounter{count: 1}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, 3, search.Indexed(), "every DB item is re-indexed on drift")
}

// Stale docs (engine has MORE than the DB, e.g. a delete dropped during an
// outage) must NOT trigger a rebuild: a re-index only upserts and can never
// shrink the count, so firing every tick would loop forever (the CPU-loop bug).
func TestReconcileSearchIndex_StaleDocs_WipesThenRebuilds(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	search := libfakes.NewFakeSearchIndex()
	// Index holds 5 docs, 3 of which are stale (a delete dropped during an outage);
	// the DB holds only 2. An upsert reindex can't shrink it, so the reconcile must
	// WIPE then rebuild from the DB, evicting the stale docs.
	for _, id := range []string{"m1", "m2", "stale1", "stale2", "stale3"} {
		require.NoError(t, search.IndexMediaItem(context.Background(),
			&library.MediaItem{ID: shared.MediaItemID(id), Title: id}))
	}

	n, err := applibrary.ReconcileSearchIndex(context.Background(), repo, search,
		fakeDocCounter{count: 5}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), search.ClearCalls.Load(), "oversized index must be wiped first")
	assert.Equal(t, 2, n, "rebuild re-indexes the DB items")
	assert.Equal(t, 2, search.Indexed(), "after wipe+rebuild only the 2 DB items remain (3 stale docs evicted)")
}

func TestReindexSearchIndex_SingleLibrary(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	seedItem(repo, "m3", "libB")
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, nil, "libA", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "only libA items counted")
	assert.Equal(t, 2, search.Indexed(), "only libA items indexed")
}

func TestReindexSearchIndex_ProgressCallback(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	search := libfakes.NewFakeSearchIndex()

	var lastDone, lastTotal int
	calls := 0
	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, nil, "", func(done, total int) {
		calls++
		lastDone, lastTotal = done, total
	})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.GreaterOrEqual(t, calls, 1)
	assert.Equal(t, 2, lastDone)
	assert.Equal(t, 2, lastTotal, "total reported matches the dataset")
}

func TestReindexSearchIndex_Empty(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, nil, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, search.Indexed())
}

func TestReindexSearchIndex_ListErrorPropagates(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Err = errors.New("db down")
	search := libfakes.NewFakeSearchIndex()

	_, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, nil, "", nil)
	require.Error(t, err)
}

func TestReindexSearchIndex_IndexErrorPropagates(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	search := libfakes.NewFakeSearchIndex()
	search.Err = errors.New("meili down")

	_, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, nil, nil, "", nil)
	require.Error(t, err)
}
