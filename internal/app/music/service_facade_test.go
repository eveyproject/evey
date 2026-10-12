package music_test

import (
	"context"
	"testing"
	"time"

	appmusic "github.com/eveyproject/evey/internal/app/music"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/music"
	musicfakes "github.com/eveyproject/evey/internal/domain/music/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Service.ListTracksByAlbum delegates to QueryService and filters by album.
func TestService_ListTracksByAlbum_FiltersByAlbum(t *testing.T) {
	t.Parallel()
	repo := musicfakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	now := time.Now()
	{
		_, uerr := repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-1", Album: "A", Title: "T1", FilePath: "/m/t1.mp3", CreatedAt: now})
		require.NoError(t, uerr)
	}
	{
		_, uerr := repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib-1", Album: "A", Title: "T2", FilePath: "/m/t2.mp3", CreatedAt: now})
		require.NoError(t, uerr)
	}
	{
		_, uerr := repo.Upsert(ctx, music.Track{ID: "t3", LibraryID: "lib-1", Album: "B", Title: "T3", FilePath: "/m/t3.mp3", CreatedAt: now})
		require.NoError(t, uerr)
	}

	scanner := musicfakes.NewFakeMusicScanner()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	tracks, err := svc.ListTracksByAlbum(ctx, "lib-1", "A")
	require.NoError(t, err)
	assert.Len(t, tracks, 2)

	tracksB, err := svc.ListTracksByAlbum(ctx, "lib-1", "B")
	require.NoError(t, err)
	assert.Len(t, tracksB, 1)
}

// FetchMissingAlbumArtAllLibraries delegates to CommandService. With no
// libRepo configured it returns 0 (no libraries to iterate).
func TestService_FetchMissingAlbumArtAllLibraries_NoLibRepo_ReturnsZero(t *testing.T) {
	t.Parallel()
	repo := musicfakes.NewFakeMusicTrackRepository()
	scanner := musicfakes.NewFakeMusicScanner()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	n, err := svc.FetchMissingAlbumArtAllLibraries(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

// Iterates libraries, skips non-music ones, and aggregates successful fetches.
// With no art fetcher wired, FetchMissingAlbumArt returns 0 per library without
// error, so the total is 0 - but the iteration is exercised (skip non-music,
// call per music lib).
func TestService_FetchMissingAlbumArtAllLibraries_IteratesMusicLibraries(t *testing.T) {
	t.Parallel()
	repo := musicfakes.NewFakeMusicTrackRepository()
	scanner := musicfakes.NewFakeMusicScanner()
	libRepo := libfakes.NewFakeLibraryQueryRepo()
	libRepo.Seed(&library.Library{ID: "lib-music-1", Name: "A", Type: library.Music})
	libRepo.Seed(&library.Library{ID: "lib-movie-1", Name: "M", Type: library.Movie})
	libRepo.Seed(&library.Library{ID: "lib-music-2", Name: "B", Type: library.Music})

	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), libRepo, scanner)
	n, err := svc.FetchMissingAlbumArtAllLibraries(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n) // no art fetcher → 0 per library
}

// AlbumArtDir returns the CommandService's configured directory.
// By default (no WithAlbumArtFetcher), it's empty string.
func TestService_AlbumArtDir_DefaultEmpty(t *testing.T) {
	t.Parallel()
	repo := musicfakes.NewFakeMusicTrackRepository()
	scanner := musicfakes.NewFakeMusicScanner()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	assert.Empty(t, svc.AlbumArtDir())
}
