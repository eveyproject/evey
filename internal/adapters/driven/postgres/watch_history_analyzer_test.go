package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchHistoryAnalyzer_GetFavoriteGenres(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	analyzer := postgres.NewWatchHistoryAnalyzer(db)
	ctx := context.Background()

	u := newTestUser("wha1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("whap1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("whalib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	// item1: Action genre - watched 3 times
	item1 := newTestMediaItem("whami1a", lib.ID)
	item1.Catalog.Genres = []string{"Action"}
	require.NoError(t, itemRepo.Create(ctx, item1))

	// item2: Drama genre - watched 1 time
	item2 := newTestMediaItem("whami1b", lib.ID)
	item2.Catalog.Genres = []string{"Drama"}
	require.NoError(t, itemRepo.Create(ctx, item2))

	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, sid := range []string{"whaps1a", "whaps1b", "whaps1c"} {
		s := newTestPlaySession(sid, string(p.ID), string(item1.ID))
		s.StartedAt = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, sessionRepo.Create(ctx, s))
	}
	s := newTestPlaySession("whaps1d", string(p.ID), string(item2.ID))
	s.StartedAt = base.Add(4 * time.Minute)
	require.NoError(t, sessionRepo.Create(ctx, s))

	// minCount=3: only "Action" qualifies
	genres, err := analyzer.GetFavoriteGenres(ctx, p.ID, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"Action"}, genres)

	// minCount=1: both qualify, Action first (higher count)
	genres2, err := analyzer.GetFavoriteGenres(ctx, p.ID, 1)
	require.NoError(t, err)
	assert.Contains(t, genres2, "Action")
	assert.Contains(t, genres2, "Drama")
}

func TestWatchHistoryAnalyzer_GetFavoriteGenres_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	analyzer := postgres.NewWatchHistoryAnalyzer(db)
	ctx := context.Background()

	u := newTestUser("wha2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("whap2", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	genres, err := analyzer.GetFavoriteGenres(ctx, p.ID, 1)
	require.NoError(t, err)
	assert.Empty(t, genres)
}

func TestWatchHistoryAnalyzer_HasWatchedSeries(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	analyzer := postgres.NewWatchHistoryAnalyzer(db)
	ctx := context.Background()

	u := newTestUser("wha3")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("whap3", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("whalib3")
	require.NoError(t, libRepo.Create(ctx, lib))

	// series (parent)
	series := newTestMediaItem("whami3series", lib.ID)
	series.Type = "series"
	require.NoError(t, itemRepo.Create(ctx, series))

	// season (parent_id = series)
	season := newTestMediaItem("whami3season", lib.ID)
	season.Type = "season"
	season.ParentID = string(series.ID)
	require.NoError(t, itemRepo.Create(ctx, season))

	// episode (parent_id = season)
	episode := newTestMediaItem("whami3ep1", lib.ID)
	episode.Type = "episode"
	episode.ParentID = string(season.ID)
	require.NoError(t, itemRepo.Create(ctx, episode))

	// no watch progress yet
	watched, err := analyzer.HasWatchedSeries(ctx, p.ID, string(series.ID))
	require.NoError(t, err)
	assert.False(t, watched)

	// mark episode as completed
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID:           "whawp3",
		ProfileID:    p.ID,
		MediaItemID:  episode.ID,
		Position:     mustWPDuration(t, 90000),
		Completed:    true,
		PlayCount:    1,
		LastPlayedAt: time.Now().UTC(),
	}))

	watched, err = analyzer.HasWatchedSeries(ctx, p.ID, string(series.ID))
	require.NoError(t, err)
	assert.True(t, watched)
}

func TestWatchHistoryAnalyzer_IsCurrentlyWatchingSeries(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	analyzer := postgres.NewWatchHistoryAnalyzer(db)
	ctx := context.Background()

	u := newTestUser("wha4")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("whap4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("whalib4")
	require.NoError(t, libRepo.Create(ctx, lib))

	series := newTestMediaItem("whami4series", lib.ID)
	series.Type = "series"
	require.NoError(t, itemRepo.Create(ctx, series))

	season := newTestMediaItem("whami4season", lib.ID)
	season.Type = "season"
	season.ParentID = string(series.ID)
	require.NoError(t, itemRepo.Create(ctx, season))

	episode := newTestMediaItem("whami4ep1", lib.ID)
	episode.Type = "episode"
	episode.ParentID = string(season.ID)
	require.NoError(t, itemRepo.Create(ctx, episode))

	// no progress yet
	inProgress, err := analyzer.IsCurrentlyWatchingSeries(ctx, p.ID, string(series.ID))
	require.NoError(t, err)
	assert.False(t, inProgress)

	// in-progress (position > 0, not completed)
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID:           "whawp4",
		ProfileID:    p.ID,
		MediaItemID:  episode.ID,
		Position:     mustWPDuration(t, 30000),
		Completed:    false,
		PlayCount:    1,
		LastPlayedAt: time.Now().UTC(),
	}))

	inProgress, err = analyzer.IsCurrentlyWatchingSeries(ctx, p.ID, string(series.ID))
	require.NoError(t, err)
	assert.True(t, inProgress)

	// once completed it should no longer be "currently watching"
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID:           "whawp4",
		ProfileID:    p.ID,
		MediaItemID:  episode.ID,
		Position:     mustWPDuration(t, 90000),
		Completed:    true,
		PlayCount:    1,
		LastPlayedAt: time.Now().UTC(),
	}))

	inProgress, err = analyzer.IsCurrentlyWatchingSeries(ctx, p.ID, string(series.ID))
	require.NoError(t, err)
	assert.False(t, inProgress)
}

func TestWatchHistoryAnalyzer_CalculateRelevanceScore(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	analyzer := postgres.NewWatchHistoryAnalyzer(db)
	ctx := context.Background()

	u := newTestUser("wha5")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("whap5", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("whalib5")
	require.NoError(t, libRepo.Create(ctx, lib))

	// item with Action + Sci-Fi genres, watched 3 times → both become favorites (minCount=3)
	item := newTestMediaItem("whami5", lib.ID)
	item.Catalog.Genres = []string{"Action", "Sci-Fi"}
	require.NoError(t, itemRepo.Create(ctx, item))

	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, sid := range []string{"whaps5a", "whaps5b", "whaps5c"} {
		s := newTestPlaySession(sid, string(p.ID), string(item.ID))
		s.StartedAt = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, sessionRepo.Create(ctx, s))
	}

	// 2 overlapping genres → score 2
	score, err := analyzer.CalculateRelevanceScore(ctx, p.ID, []string{"Action", "Sci-Fi", "Drama"})
	require.NoError(t, err)
	assert.Equal(t, 2, score)

	// no overlap
	score, err = analyzer.CalculateRelevanceScore(ctx, p.ID, []string{"Romance", "Horror"})
	require.NoError(t, err)
	assert.Equal(t, 0, score)

	// empty genres slice → always 0
	score, err = analyzer.CalculateRelevanceScore(ctx, p.ID, []string{})
	require.NoError(t, err)
	assert.Equal(t, 0, score)
}

// The batch path joined the WHOLE of play_sessions per profile while the
// single-profile one capped at the 200 most recent sessions - so on a long
// history the two disagreed, and the batch cost grew forever (the retention
// purge for that table is off by default). They now share one window; this
// asserts they agree, and that each profile gets its OWN window rather than a
// global one.
func TestWatchHistoryAnalyzer_GetFavoriteGenresBatch_MatchesSingleProfile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	analyzer := postgres.NewWatchHistoryAnalyzer(db)
	ctx := context.Background()

	u := newTestUser("whab")
	require.NoError(t, userRepo.Create(ctx, u))
	pa := newTestProfile("whabpa", u.ID)
	require.NoError(t, profileRepo.Create(ctx, pa))
	pb := newTestProfile("whabpb", u.ID)
	require.NoError(t, profileRepo.Create(ctx, pb))
	lib := newTestLibrary("whablib")
	require.NoError(t, libRepo.Create(ctx, lib))

	action := newTestMediaItem("whabmi1", lib.ID)
	action.Catalog.Genres = []string{"Action"}
	require.NoError(t, itemRepo.Create(ctx, action))
	drama := newTestMediaItem("whabmi2", lib.ID)
	drama.Catalog.Genres = []string{"Drame"}
	require.NoError(t, itemRepo.Create(ctx, drama))

	base := time.Now().UTC().Truncate(time.Millisecond)
	// Profile A: 3 Action. Profile B: 2 Drame. Together they exceed no window,
	// but a GLOBAL window instead of a per-profile one would mix them up.
	for i := range 3 {
		s := newTestPlaySession(fmt.Sprintf("whabsa%d", i), string(pa.ID), string(action.ID))
		s.StartedAt = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, sessionRepo.Create(ctx, s))
	}
	for i := range 2 {
		s := newTestPlaySession(fmt.Sprintf("whabsb%d", i), string(pb.ID), string(drama.ID))
		s.StartedAt = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, sessionRepo.Create(ctx, s))
	}

	batch, err := analyzer.GetFavoriteGenresBatch(ctx, []shared.ProfileID{pa.ID, pb.ID}, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"Action"}, batch[pa.ID])
	assert.Equal(t, []string{"Drame"}, batch[pb.ID], "each profile keeps its own window")

	// Same answer as the single-profile query, which is the whole point.
	single, err := analyzer.GetFavoriteGenres(ctx, pa.ID, 2)
	require.NoError(t, err)
	assert.Equal(t, single, batch[pa.ID])
}
