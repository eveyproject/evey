package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChapterRepo_ReplaceBatch_GetByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	chRepo := postgres.NewChapterRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("chlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("chmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	chapters := []*library.Chapter{
		{ID: "ch1", MediaItemID: item.ID, Title: "Intro", StartTimeMs: 0, EndTimeMs: 90000, SortOrder: 0},
		{ID: "ch2", MediaItemID: item.ID, Title: "Act 1", StartTimeMs: 90000, EndTimeMs: 300000, SortOrder: 1},
		{ID: "ch3", MediaItemID: item.ID, Title: "Credits", StartTimeMs: 300000, EndTimeMs: 360000, SortOrder: 2},
	}
	require.NoError(t, chRepo.ReplaceBatch(ctx, item.ID, chapters))

	got, err := chRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Equal(t, "ch1", got[0].ID)
	assert.Equal(t, "Intro", got[0].Title)
	assert.Equal(t, int64(0), got[0].StartTimeMs)
	assert.Equal(t, int64(90000), got[0].EndTimeMs)

	assert.Equal(t, "ch3", got[2].ID)
	assert.Equal(t, 2, got[2].SortOrder)
}

func TestChapterRepo_ReplaceBatch_OverwriteExisting(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	chRepo := postgres.NewChapterRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("chlib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("chmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	initial := []*library.Chapter{
		{ID: "old1", MediaItemID: item.ID, Title: "Old Chapter", StartTimeMs: 0, EndTimeMs: 1000, SortOrder: 0},
	}
	require.NoError(t, chRepo.ReplaceBatch(ctx, item.ID, initial))

	replacement := []*library.Chapter{
		{ID: "new1", MediaItemID: item.ID, Title: "New Chapter A", StartTimeMs: 0, EndTimeMs: 500, SortOrder: 0},
		{ID: "new2", MediaItemID: item.ID, Title: "New Chapter B", StartTimeMs: 500, EndTimeMs: 1000, SortOrder: 1},
	}
	require.NoError(t, chRepo.ReplaceBatch(ctx, item.ID, replacement))

	got, err := chRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "new1", got[0].ID)
	assert.Equal(t, "new2", got[1].ID)
}

func TestChapterRepo_ReplaceBatch_EmptyClears(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	chRepo := postgres.NewChapterRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("chlib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("chmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	chapters := []*library.Chapter{
		{ID: "clear1", MediaItemID: item.ID, Title: "To Remove", StartTimeMs: 0, EndTimeMs: 1000, SortOrder: 0},
	}
	require.NoError(t, chRepo.ReplaceBatch(ctx, item.ID, chapters))

	require.NoError(t, chRepo.ReplaceBatch(ctx, item.ID, nil))

	got, err := chRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}
