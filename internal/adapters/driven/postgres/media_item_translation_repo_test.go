package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
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
