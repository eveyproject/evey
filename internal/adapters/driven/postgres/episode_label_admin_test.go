package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

const wantEpisodeLabel = "Dark: Show: Part 2 · S02E03 · The Reveal"

type labelFixture struct {
	episodeID string
	movieID   string
	profileID string
}

// seedLabelFixture stores a Series > Season 2 > Episode 3 chain and a movie,
// each with one play session by one profile.
func seedLabelFixture(t *testing.T, db *sql.DB, prefix string) labelFixture {
	t.Helper()
	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)

	u := newTestUser(prefix + "-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile(prefix+"-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary(prefix + "-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	series := newTestMediaItem(prefix+"-series", lib.ID)
	series.Type, series.Title = library.Series, "Dark: Show: Part 2"
	require.NoError(t, itemRepo.Create(ctx, series))

	two, three := 2, 3
	season := newTestMediaItem(prefix+"-season", lib.ID)
	season.Type, season.Title, season.ParentID, season.IndexNumber = library.Season, "Season 2", string(series.ID), &two
	require.NoError(t, itemRepo.Create(ctx, season))

	ep := newTestMediaItem(prefix+"-ep", lib.ID)
	ep.Type, ep.Title, ep.ParentID, ep.IndexNumber = library.Episode, "The Reveal", string(season.ID), &three
	require.NoError(t, itemRepo.Create(ctx, ep))

	movie := newTestMediaItem(prefix+"-movie", lib.ID)
	movie.Title = "Plain Movie"
	require.NoError(t, itemRepo.Create(ctx, movie))

	s1 := newTestPlaySession(prefix+"-ps-ep", string(p.ID), string(ep.ID))
	s1.StartedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, sessionRepo.Create(ctx, s1))
	s2 := newTestPlaySession(prefix+"-ps-movie", string(p.ID), string(movie.ID))
	s2.StartedAt = s1.StartedAt.Add(-time.Minute)
	require.NoError(t, sessionRepo.Create(ctx, s2))
	return labelFixture{episodeID: string(ep.ID), movieID: string(movie.ID), profileID: string(p.ID)}
}

func TestPlayHistoryRepo_List_EpisodeLabel(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	f := seedLabelFixture(t, db, "lbl1")

	entries, err := postgres.NewPlayHistoryRepository(db).List(context.Background(),
		activity.PlayHistoryFilter{ProfileID: shared.ProfileID(f.profileID)})
	require.NoError(t, err)
	titles := map[string]string{}
	for _, e := range entries {
		titles[string(e.MediaItemID)] = e.MediaTitle
	}
	assert.Equal(t, wantEpisodeLabel, titles[f.episodeID])
	assert.Equal(t, "Plain Movie", titles[f.movieID])
}

func TestAnalyticsReader_EpisodeLabel(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedLabelFixture(t, db, "lbl2")

	data, err := postgres.NewAnalyticsReader(db).GetAnalytics(context.Background())
	require.NoError(t, err)
	var recent, top []string
	for _, r := range data.RecentActivity {
		recent = append(recent, r.MediaTitle)
	}
	for _, m := range data.TopMedia {
		top = append(top, m.Title)
	}
	assert.ElementsMatch(t, []string{wantEpisodeLabel, "Plain Movie"}, recent)
	assert.ElementsMatch(t, []string{wantEpisodeLabel, "Plain Movie"}, top)
}

func TestAuditLogRepo_List_EpisodeLabel(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	f := seedLabelFixture(t, db, "lbl3")
	repo := postgres.NewAuditLogRepository(db)
	ctx := context.Background()

	for i, id := range []string{f.episodeID, f.movieID} {
		require.NoError(t, repo.Create(ctx, &activity.AuditLogEntry{
			ID: "lbl3-al-" + string(rune('a'+i)), UserID: "lbl3-u", Action: "edit",
			ResourceType: "media", ResourceID: new(id), Details: map[string]any{},
			CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		}))
	}
	entries, err := repo.List(ctx, activity.AuditLogFilter{Limit: 10})
	require.NoError(t, err)
	names := map[string]string{}
	for _, e := range entries {
		names[*e.ResourceID] = e.ResourceName
	}
	assert.Equal(t, wantEpisodeLabel, names[f.episodeID])
	assert.Equal(t, "Plain Movie", names[f.movieID])
}

func TestPlaySessionStatsReader_MostPlayed_EpisodeLabel(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	f := seedLabelFixture(t, db, "lbl4")

	stats, err := postgres.NewPlaySessionStatsReader(db).GetUserStats(context.Background(), shared.ProfileID(f.profileID))
	require.NoError(t, err)
	var titles []string
	for _, m := range stats.MostPlayedItems {
		titles = append(titles, m.Title)
	}
	assert.ElementsMatch(t, []string{wantEpisodeLabel, "Plain Movie"}, titles)
}

func TestPlaySessionStatsReader_GetServerStats_PopularMedia_EpisodeLabel(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedLabelFixture(t, db, "lbl5")

	stats, err := postgres.NewPlaySessionStatsReader(db).GetServerStats(context.Background())
	require.NoError(t, err)
	var titles []string
	for _, m := range stats.PopularMedia {
		titles = append(titles, m.Title)
	}
	assert.ElementsMatch(t, []string{wantEpisodeLabel, "Plain Movie"}, titles)
}

func TestPlaybackQoEReader_GetTopStarvedTitles_EpisodeLabel(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	itemRepo := postgres.NewMediaItemRepository(db)

	lib := newTestLibrary("lbl6-lib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	series := newTestMediaItem(uuid.NewString(), lib.ID)
	series.Type, series.Title = library.Series, "Dark: Show: Part 2"
	require.NoError(t, itemRepo.Create(ctx, series))
	two, three := 2, 3
	season := newTestMediaItem(uuid.NewString(), lib.ID)
	season.Type, season.Title, season.ParentID, season.IndexNumber = library.Season, "Season 2", string(series.ID), &two
	require.NoError(t, itemRepo.Create(ctx, season))
	ep := newTestMediaItem(uuid.NewString(), lib.ID)
	ep.Type, ep.Title, ep.ParentID, ep.IndexNumber = library.Episode, "The Reveal", string(season.ID), &three
	require.NoError(t, itemRepo.Create(ctx, ep))
	movie := newTestMediaItem(uuid.NewString(), lib.ID)
	movie.Title = "Plain Movie"
	require.NoError(t, itemRepo.Create(ctx, movie))

	repo := postgres.NewPlaybackQoERepo(db)
	for _, id := range []string{string(ep.ID), string(movie.ID)} {
		for range 60 {
			mediaID, buf, kbps := id, 2000, 6000
			require.NoError(t, repo.Insert(ctx, postgres.QoEEventRow{
				MediaItemID: &mediaID, SessionID: "s", ContentID: mediaID, ObjectType: "v", StreamingFormat: "h", StreamType: "v",
				BufferStarved: true, BufferLengthMs: &buf, EncodedKbps: &kbps, DeviceClass: "tv", Raw: map[string]string{},
			}))
		}
	}

	top, err := postgres.NewPlaybackQoEReader(db).GetTopStarvedTitles(ctx, 24*time.Hour, 10)
	require.NoError(t, err)
	var titles []string
	for _, r := range top {
		titles = append(titles, r.Title)
	}
	assert.ElementsMatch(t, []string{wantEpisodeLabel, "Plain Movie"}, titles)
}
