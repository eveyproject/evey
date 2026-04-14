package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/subtitle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubtitleRepo_Save_ListByMediaItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	subRepo := postgres.NewSubtitleRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("sublib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("submi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	now := time.Now().UTC().Truncate(time.Millisecond)
	sub1 := &subtitle.SubtitleDownload{
		ID: "sub1", MediaItemID: item.ID, Language: "en", Format: "srt",
		FilePath: "/subs/en.srt", Source: "opensubtitles", DownloadedAt: now,
	}
	sub2 := &subtitle.SubtitleDownload{
		ID: "sub2", MediaItemID: item.ID, Language: "fr", Format: "ass",
		FilePath: "/subs/fr.ass", Source: "opensubtitles", DownloadedAt: now.Add(time.Second),
	}
	require.NoError(t, subRepo.Save(ctx, sub1))
	require.NoError(t, subRepo.Save(ctx, sub2))

	got, err := subRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Ordered by downloaded_at DESC, so sub2 (fr) first
	assert.Equal(t, "sub2", got[0].ID)
	assert.Equal(t, "fr", got[0].Language)
	assert.Equal(t, "sub1", got[1].ID)
	assert.Equal(t, "en", got[1].Language)
}

func TestSubtitleRepo_Save_Upsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	subRepo := postgres.NewSubtitleRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("sublib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("submi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	sub := &subtitle.SubtitleDownload{
		ID: "subu1", MediaItemID: item.ID, Language: "en", Format: "srt",
		FilePath: "/subs/old.srt", Source: "opensubtitles", DownloadedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, subRepo.Save(ctx, sub))

	sub.FilePath = "/subs/new.srt"
	sub.Language = "de"
	require.NoError(t, subRepo.Save(ctx, sub))

	got, err := subRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "/subs/new.srt", got[0].FilePath)
	assert.Equal(t, "de", got[0].Language)
}

func TestSubtitleRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	subRepo := postgres.NewSubtitleRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("sublib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("submi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	sub := &subtitle.SubtitleDownload{
		ID: "subdel1", MediaItemID: item.ID, Language: "en", Format: "srt",
		FilePath: "/subs/en.srt", Source: "opensubtitles", DownloadedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, subRepo.Save(ctx, sub))
	require.NoError(t, subRepo.Delete(ctx, sub.ID))

	got, err := subRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSubtitleRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	subRepo := postgres.NewSubtitleRepository(db)
	err := subRepo.Delete(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}
