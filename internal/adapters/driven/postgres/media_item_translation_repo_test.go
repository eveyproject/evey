package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaItemTranslationRepo_Upsert_Get(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaItemTranslationRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	tr := &library.MediaItemTranslation{MediaItemID: item.ID, Locale: "fr-FR", Title: "Titre", Overview: "Résumé", Tagline: "Slogan"}
	require.NoError(t, repo.Upsert(ctx, tr))

	got, err := repo.GetByMediaItemAndLocale(ctx, string(item.ID), "fr-FR")
	require.NoError(t, err)
	assert.Equal(t, "Titre", got.Title)
	assert.Equal(t, "Résumé", got.Overview)
	assert.Equal(t, "Slogan", got.Tagline)

	// Upsert overwrites
	tr2 := &library.MediaItemTranslation{MediaItemID: item.ID, Locale: "fr-FR", Title: "Nouveau", Overview: "O", Tagline: "T"}
	require.NoError(t, repo.Upsert(ctx, tr2))
	got2, err := repo.GetByMediaItemAndLocale(ctx, string(item.ID), "fr-FR")
	require.NoError(t, err)
	assert.Equal(t, "Nouveau", got2.Title)
}

func TestMediaItemTranslationRepo_GetByMediaItemAndLocale_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewMediaItemTranslationRepository(db)
	_, err := repo.GetByMediaItemAndLocale(context.Background(), "no-such", "en-US")
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

func TestMediaItemTranslationRepo_GetByMediaItemsAndLocale(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaItemTranslationRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tlib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	a := newTestMediaItem("tmi-a", lib.ID)
	b := newTestMediaItem("tmi-b", lib.ID)
	c := newTestMediaItem("tmi-c", lib.ID)
	for _, it := range []*library.MediaItem{a, b, c} {
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	require.NoError(t, repo.Upsert(ctx, &library.MediaItemTranslation{MediaItemID: a.ID, Locale: "fr-FR", Title: "Titre A"}))
	require.NoError(t, repo.Upsert(ctx, &library.MediaItemTranslation{MediaItemID: b.ID, Locale: "fr-FR", Title: "Titre B"}))
	require.NoError(t, repo.Upsert(ctx, &library.MediaItemTranslation{MediaItemID: a.ID, Locale: "de-DE", Title: "Titel A"}))

	// fr-FR over {a,b,c}: a+b present, c absent - and absence is NOT an error.
	got, err := repo.GetByMediaItemsAndLocale(ctx, []string{string(a.ID), string(b.ID), string(c.ID)}, "fr-FR")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Titre A", got[string(a.ID)].Title)
	assert.Equal(t, "Titre B", got[string(b.ID)].Title)
	_, hasC := got[string(c.ID)]
	assert.False(t, hasC)

	// Locale isolation: de-DE returns only a's German title.
	gotDE, err := repo.GetByMediaItemsAndLocale(ctx, []string{string(a.ID), string(b.ID)}, "de-DE")
	require.NoError(t, err)
	require.Len(t, gotDE, 1)
	assert.Equal(t, "Titel A", gotDE[string(a.ID)].Title)

	// Empty inputs → empty map, no error (no query issued).
	empty, err := repo.GetByMediaItemsAndLocale(ctx, nil, "fr-FR")
	require.NoError(t, err)
	assert.Empty(t, empty)
	empty2, err := repo.GetByMediaItemsAndLocale(ctx, []string{string(a.ID)}, "")
	require.NoError(t, err)
	assert.Empty(t, empty2)
}

func TestMediaItemTranslationRepo_GetByMediaItem_OrderedAndDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaItemTranslationRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tlib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	for _, loc := range []string{"fr-FR", "en-US", "de-DE"} {
		require.NoError(t, repo.Upsert(ctx, &library.MediaItemTranslation{MediaItemID: item.ID, Locale: loc, Title: "t-" + loc}))
	}

	list, err := repo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	require.Len(t, list, 3)
	// Ordered by locale ASC
	assert.Equal(t, "de-DE", list[0].Locale)
	assert.Equal(t, "en-US", list[1].Locale)
	assert.Equal(t, "fr-FR", list[2].Locale)

	require.NoError(t, repo.DeleteByMediaItem(ctx, string(item.ID)))
	list2, err := repo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Empty(t, list2)
}

// Enrichment writes 5-10 locales per item in ONE statement, and the search
// reindex reads every locale of a page of items in one query, grouped by
// item and ordered by locale.
func TestMediaItemTranslationRepo_UpsertBatch_GetByMediaItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("tblib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	a, b := newTestMediaItem("tb-a", lib.ID), newTestMediaItem("tb-b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, a))
	require.NoError(t, itemRepo.Create(ctx, b))
	repo := postgres.NewMediaItemTranslationRepository(db)

	require.NoError(t, repo.UpsertBatch(ctx, nil), "an empty batch is a no-op")
	require.NoError(t, repo.UpsertBatch(ctx, []*library.MediaItemTranslation{
		{MediaItemID: a.ID, Locale: "fr-FR", Title: "Titre A", Overview: "R", Tagline: "S"},
		{MediaItemID: a.ID, Locale: "de-DE", Title: "Titel A"},
		{MediaItemID: b.ID, Locale: "fr-FR", Title: "Titre B"},
	}))
	// A second batch rewrites an existing locale in place.
	require.NoError(t, repo.UpsertBatch(ctx, []*library.MediaItemTranslation{
		{MediaItemID: a.ID, Locale: "fr-FR", Title: "Titre A v2", Overview: "R2", Tagline: "S2"},
	}))

	got, err := repo.GetByMediaItems(ctx, []string{string(a.ID), string(b.ID), "ghost"})
	require.NoError(t, err)
	require.Len(t, got[string(a.ID)], 2)
	assert.Equal(t, "de-DE", got[string(a.ID)][0].Locale, "locales come back ordered")
	assert.Equal(t, "Titre A v2", got[string(a.ID)][1].Title)
	assert.Equal(t, "R2", got[string(a.ID)][1].Overview)
	require.Len(t, got[string(b.ID)], 1)
	assert.NotContains(t, got, "ghost")
	none, err := repo.GetByMediaItems(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
