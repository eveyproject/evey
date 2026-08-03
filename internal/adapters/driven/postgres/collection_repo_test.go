package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCollection(id string) *library.Collection {
	now := time.Now().UTC().Truncate(time.Millisecond)
	tmdbID := 12345
	return &library.Collection{
		ID:          id,
		Name:        "Collection " + id,
		Overview:    "Overview for " + id,
		TmdbID:      &tmdbID,
		PosterURL:   "https://image.tmdb.org/t/p/w500/poster.jpg",
		BackdropURL: "https://image.tmdb.org/t/p/original/backdrop.jpg",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestCollectionRepo_Create(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col1")
	col.TmdbID = nil // allow nil tmdb_id
	require.NoError(t, repo.Create(ctx, col))

	got, err := repo.GetByID(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, col.Name, got.Name)
	assert.Equal(t, col.Overview, got.Overview)
}

func TestCollectionRepo_Upsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col_upsert")
	require.NoError(t, repo.Upsert(ctx, col))

	// upsert same tmdb_id with updated name
	col2 := newTestCollection("col_upsert_2")
	col2.TmdbID = col.TmdbID
	col2.Name = "Updated Name"
	require.NoError(t, repo.Upsert(ctx, col2))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "Updated Name", list[0].Name)

	// RETURNING id: the conflict path must yield the SURVIVING row id, not col2's
	// freshly-generated uuid - else enricher LinkMediaItem(col.ID, …) targets a
	// non-existent collection and the FK silently drops every link after the
	// first enrichment of a TMDB collection.
	assert.Equal(t, col.ID, col2.ID, "upsert conflict must return the existing row id, not a new uuid")
	assert.Equal(t, col.ID, list[0].ID)
}

func TestCollectionRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	c1 := newTestCollection("col2a")
	c1.TmdbID = nil
	c2 := newTestCollection("col2b")
	c2.TmdbID = nil
	require.NoError(t, repo.Create(ctx, c1))
	require.NoError(t, repo.Create(ctx, c2))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestCollectionRepo_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col3")
	require.NoError(t, repo.Create(ctx, col))

	got, err := repo.GetByID(ctx, "col3")
	require.NoError(t, err)
	assert.Equal(t, col.Name, got.Name)
	assert.Equal(t, col.Overview, got.Overview)
	assert.NotNil(t, got.TmdbID)
	assert.Equal(t, *col.TmdbID, *got.TmdbID)

	_, err = repo.GetByID(ctx, "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestCollectionRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col5")
	require.NoError(t, repo.Create(ctx, col))
	require.NoError(t, repo.Delete(ctx, col.ID))

	_, err := repo.GetByID(ctx, col.ID)
	require.ErrorIs(t, err, shared.ErrNotFound)

	err = repo.Delete(ctx, "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestCollectionRepo_GetItemsByCollectionIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("batchlib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	// Give distinct premiere dates so ORDER BY is deterministic.
	p1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	p2 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	p3 := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

	item1 := newTestMediaItem("batchmi1", lib.ID)
	item1.Catalog.PremiereDate = &p1
	item2 := newTestMediaItem("batchmi2", lib.ID)
	item2.Catalog.PremiereDate = &p2
	item3 := newTestMediaItem("batchmi3", lib.ID)
	item3.Catalog.PremiereDate = &p3
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))
	require.NoError(t, itemRepo.Create(ctx, item3))

	col1 := newTestCollection("batchcol1")
	col1.TmdbID = nil
	col2 := newTestCollection("batchcol2")
	col2.TmdbID = nil
	require.NoError(t, colRepo.Create(ctx, col1))
	require.NoError(t, colRepo.Create(ctx, col2))

	// col1 gets item1 + item2; col2 gets item3.
	require.NoError(t, colRepo.LinkItem(ctx, col1.ID, string(item1.ID), 0))
	require.NoError(t, colRepo.LinkItem(ctx, col1.ID, string(item2.ID), 1))
	require.NoError(t, colRepo.LinkItem(ctx, col2.ID, string(item3.ID), 0))

	// Batch fetch both collections at once.
	batch, err := colRepo.GetItemsByCollectionIDs(ctx, []string{col1.ID, col2.ID})
	require.NoError(t, err)

	// Individual fetches for comparison.
	ind1, indErr1 := colRepo.GetItems(ctx, col1.ID, 100, 0)
	require.NoError(t, indErr1)
	ind2, indErr2 := colRepo.GetItems(ctx, col2.ID, 100, 0)
	require.NoError(t, indErr2)

	// Batch must return the same IDs in the same order as individual GetItems.
	batchIDs1 := idsFrom(batch[col1.ID])
	batchIDs2 := idsFrom(batch[col2.ID])
	assert.Equal(t, idsFrom(ind1), batchIDs1, "col1: batch must match GetItems order")
	assert.Equal(t, idsFrom(ind2), batchIDs2, "col2: batch must match GetItems order")
	assert.Len(t, batchIDs1, 2)
	assert.Len(t, batchIDs2, 1)

	// Empty input must return empty map, no DB round-trip.
	emptyResult, emptyErr := colRepo.GetItemsByCollectionIDs(ctx, nil)
	require.NoError(t, emptyErr)
	assert.Empty(t, emptyResult)

	// Unknown collection ID must produce an empty (missing) entry, not an error.
	unknownResult, unknownErr := colRepo.GetItemsByCollectionIDs(ctx, []string{"col-does-not-exist"})
	require.NoError(t, unknownErr)
	assert.Empty(t, unknownResult["col-does-not-exist"])
}

// idsFrom returns the string IDs of a MediaItem slice in order.
func idsFrom(items []*library.MediaItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = string(it.ID)
	}
	return out
}

func TestCollectionRepo_GetItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("collib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item1 := newTestMediaItem("colmi1", lib.ID)
	item2 := newTestMediaItem("colmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))

	col := newTestCollection("colsaga1")
	col.TmdbID = nil
	require.NoError(t, colRepo.Create(ctx, col))

	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(item1.ID), 0))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(item2.ID), 1))

	items, err := colRepo.GetItems(ctx, col.ID, 100, 0)
	require.NoError(t, err)
	assert.Len(t, items, 2)

	count, err := colRepo.CountItems(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestCollectionSmartQuerier_QueryByRulesPaginatesAndCounts(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	smartQ := postgres.NewCollectionSmartQuerier(db)
	ctx := context.Background()

	lib := newTestLibrary("smartlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	for _, id := range []string{"sm1", "sm2", "sm3", "sm4", "sm5"} {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(id, lib.ID)))
	}

	rules := []library.CollectionRule{{Field: "type", Operator: "eq", Value: "movie"}}

	total, err := smartQ.CountByRules(ctx, rules)
	require.NoError(t, err)
	assert.Equal(t, 5, total, "CountByRules returns the full match count, uncapped")

	page, err := smartQ.QueryByRules(ctx, rules, "added_at", "asc", 2, 2)
	require.NoError(t, err)
	assert.Len(t, page, 2, "LIMIT 2 OFFSET 2 returns a 2-item page out of 5")
}

// The saga-repair work list: movies carrying a provider id that belong to no
// collection. This is what the field complaint reduced to - the sequels were
// in the library, the saga listed one title.
func TestCollectionRepo_ListMoviesWithoutCollection(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("unlinklib")
	require.NoError(t, libRepo.Create(ctx, lib))

	linked := newTestMediaItem("unlinked-a", lib.ID)
	orphan := newTestMediaItem("unlinked-b", lib.ID)
	noProvider := newTestMediaItem("unlinked-c", lib.ID)
	noProvider.Catalog.ProviderIDs = map[string]string{}
	episode := newTestMediaItem("unlinked-d", lib.ID)
	episode.Type = library.Episode
	for _, it := range []*library.MediaItem{linked, orphan, noProvider, episode} {
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	col := newTestCollection("unlinkcol")
	col.TmdbID = nil
	require.NoError(t, colRepo.Create(ctx, col))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(linked.ID), 0))

	got, err := colRepo.ListMoviesWithoutCollection(ctx, 100)
	require.NoError(t, err)

	ids := map[string]string{}
	for _, m := range got {
		ids[string(m.MediaItemID)] = m.ProviderID
	}
	assert.Contains(t, ids, "unlinked-b", "an unlinked movie with a tmdb id is work to do")
	assert.Equal(t, "123", ids["unlinked-b"], "the provider id must come back with it")
	assert.NotContains(t, ids, "unlinked-a", "an already-linked movie is done")
	assert.NotContains(t, ids, "unlinked-c", "no provider id, nothing to resolve it from")
	assert.NotContains(t, ids, "unlinked-d", "TMDB collections are a movie notion")
}

// Membership must be removable: AddCollectionItem shipped without a
// counterpart, so a wrong member was permanent.
func TestCollectionRepo_UnlinkItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("unlink2lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	a := newTestMediaItem("unlink2-a", lib.ID)
	b := newTestMediaItem("unlink2-b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, a))
	require.NoError(t, itemRepo.Create(ctx, b))
	col := newTestCollection("unlink2col")
	col.TmdbID = nil
	require.NoError(t, colRepo.Create(ctx, col))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(a.ID), 0))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(b.ID), 1))

	require.NoError(t, colRepo.UnlinkItem(ctx, col.ID, string(a.ID)))
	items, err := colRepo.GetItems(ctx, col.ID, 100, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, b.ID, items[0].ID)

	// Removing what is not there is the state the caller asked for.
	require.NoError(t, colRepo.UnlinkItem(ctx, col.ID, "never-linked"))
}

// A "manual" collection reads in its hand-set order - the only way to express
// a chronology that differs from the release order (prequels). Until now
// sort_order was written by LinkItem and read by nobody.
func TestCollectionRepo_ManualOrderIsHonoured(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("orderlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	// Release order: old (1977) then recent (2005). Chronological order of the
	// story is the opposite - exactly the prequel case.
	old := newTestMediaItem("order-old", lib.ID)
	old.Catalog.PremiereDate = new(time.Date(1977, 5, 25, 0, 0, 0, 0, time.UTC))
	recent := newTestMediaItem("order-recent", lib.ID)
	recent.Catalog.PremiereDate = new(time.Date(2005, 5, 19, 0, 0, 0, 0, time.UTC))
	require.NoError(t, itemRepo.Create(ctx, old))
	require.NoError(t, itemRepo.Create(ctx, recent))

	col := newTestCollection("ordercol")
	col.TmdbID = nil
	col.SortBy = "manual"
	require.NoError(t, colRepo.Create(ctx, col))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(old.ID), 0))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(recent.ID), 0))

	require.NoError(t, colRepo.SetItemOrder(ctx, col.ID, []string{string(recent.ID), string(old.ID)}))
	items, err := colRepo.GetItems(ctx, col.ID, 100, 0)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, recent.ID, items[0].ID, "the hand-set order must win over the release date")
	assert.Equal(t, old.ID, items[1].ID)
}

// A collection that is not "manual" keeps reading in release order, whatever
// sort_order happens to hold - the default must not change under anyone.
func TestCollectionRepo_ReleaseOrderStaysTheDefault(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("relorderlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	old := newTestMediaItem("rel-old", lib.ID)
	old.Catalog.PremiereDate = new(time.Date(1979, 5, 25, 0, 0, 0, 0, time.UTC))
	recent := newTestMediaItem("rel-recent", lib.ID)
	recent.Catalog.PremiereDate = new(time.Date(2024, 8, 16, 0, 0, 0, 0, time.UTC))
	require.NoError(t, itemRepo.Create(ctx, old))
	require.NoError(t, itemRepo.Create(ctx, recent))

	col := newTestCollection("relordercol")
	col.TmdbID = nil // sort_by defaults to added_at, i.e. NOT manual
	require.NoError(t, colRepo.Create(ctx, col))
	// Deliberately hostile hand positions: they must be ignored here.
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(recent.ID), 1))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, string(old.ID), 2))

	items, err := colRepo.GetItems(ctx, col.ID, 100, 0)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, old.ID, items[0].ID, "release order stays the default reading order")
	assert.Equal(t, recent.ID, items[1].ID)
}
