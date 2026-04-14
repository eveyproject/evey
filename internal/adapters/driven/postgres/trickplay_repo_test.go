package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrickplayRepo_Create_GetByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tplib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tpmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	now := time.Now().UTC().Truncate(time.Millisecond)
	m := &library.TrickplayManifest{
		ID:          "trick1",
		MediaItemID: item.ID,
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  25,
		ImagePath:   "/trickplay/tpmi1",
		CreatedAt:   now,
	}
	require.NoError(t, tpRepo.Create(ctx, m))

	got, err := tpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)
	assert.Equal(t, m.MediaItemID, got.MediaItemID)
	assert.Equal(t, m.Interval, got.Interval)
	assert.Equal(t, m.Width, got.Width)
	assert.Equal(t, m.Height, got.Height)
	assert.Equal(t, m.TileWidth, got.TileWidth)
	assert.Equal(t, m.TileHeight, got.TileHeight)
	assert.Equal(t, m.ImageCount, got.ImageCount)
	assert.Equal(t, m.ImagePath, got.ImagePath)
	assert.Equal(t, m.CreatedAt, got.CreatedAt)
}

func TestTrickplayRepo_Create_Upsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tplib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tpmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	first := &library.TrickplayManifest{
		ID:          "trick2a",
		MediaItemID: item.ID,
		Interval:    5,
		Width:       160,
		Height:      90,
		TileWidth:   4,
		TileHeight:  4,
		ImageCount:  16,
		ImagePath:   "/trickplay/tpmi2/v1",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, tpRepo.Create(ctx, first))

	updated := &library.TrickplayManifest{
		ID:          "trick2b",
		MediaItemID: item.ID,
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  25,
		ImagePath:   "/trickplay/tpmi2/v2",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, tpRepo.Create(ctx, updated))

	got, err := tpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, updated.Interval, got.Interval)
	assert.Equal(t, updated.Width, got.Width)
	assert.Equal(t, updated.ImageCount, got.ImageCount)
	assert.Equal(t, updated.ImagePath, got.ImagePath)
}

func TestTrickplayRepo_GetByMediaItemID_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	_, err := tpRepo.GetByMediaItemID(ctx, "nonexistent")
	assert.Equal(t, shared.ErrNotFound, err)
}

func TestTrickplayRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tplib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tpmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	m := &library.TrickplayManifest{
		ID:          "trick3",
		MediaItemID: item.ID,
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  25,
		ImagePath:   "/trickplay/tpmi3",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, tpRepo.Create(ctx, m))

	require.NoError(t, tpRepo.Delete(ctx, item.ID))

	_, err := tpRepo.GetByMediaItemID(ctx, item.ID)
	assert.Equal(t, shared.ErrNotFound, err)
}
