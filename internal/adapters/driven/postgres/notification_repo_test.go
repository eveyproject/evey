package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedNotifProfile seeds the profile plus the "media-1" title the fixtures
// point at: a notification whose media item is gone is hidden by design.
func seedNotifProfile(t *testing.T, db *sql.DB, profileID, userID string) {
	t.Helper()
	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	require.NoError(t, userRepo.Create(ctx, newTestUser(userID)))
	require.NoError(t, profileRepo.Create(ctx, newTestProfile(profileID, shared.UserID(userID))))
	seedNotifMedia(t, db, "media-1")
}

func seedNotifMedia(t *testing.T, db *sql.DB, mediaID string) {
	t.Helper()
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	if _, err := libRepo.GetByID(ctx, "notif-lib"); err != nil {
		require.NoError(t, libRepo.Create(ctx, newTestLibrary("notif-lib")))
	}
	// Idempotent: a test that seeds two profiles seeds the title twice.
	if _, err := itemRepo.GetByID(ctx, shared.MediaItemID(mediaID)); err == nil {
		return
	}
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(mediaID, "notif-lib")))
}

// A deleted or re-imported title leaves its notifications behind (no foreign
// key on media_item_id, on purpose). Those must neither be listed nor counted
// - tapping one opened a 404 with a useless retry - and the daily sweep removes
// them. A notification about nothing in particular (empty media id) stays.
func TestNotificationRepo_HidesAndSweepsOrphans(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedNotifProfile(t, db, "pO", "notif-u9")
	seedNotifMedia(t, db, "media-gone")
	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()

	mk := func(id, mediaID string) *notification.Notification {
		return &notification.Notification{
			ID: id, ProfileID: "pO", Type: notification.TypeNewMovie, Title: id,
			MediaItemID: shared.MediaItemID(mediaID), Level: notification.LevelAll, CreatedAt: time.Now(),
		}
	}
	require.NoError(t, repo.Create(ctx, mk("alive", "media-1")))
	require.NoError(t, repo.Create(ctx, mk("orphan", "media-gone")))
	require.NoError(t, repo.Create(ctx, mk("no-media", "")))
	require.NoError(t, postgres.NewMediaItemRepository(db).Delete(ctx, "media-gone"))

	list, err := repo.ListByProfile(ctx, "pO", false, 10, 0)
	require.NoError(t, err)
	ids := make([]string, 0, len(list))
	for _, n := range list {
		ids = append(ids, n.ID)
	}
	assert.ElementsMatch(t, []string{"alive", "no-media"}, ids)

	count, err := repo.UnreadCount(ctx, "pO")
	require.NoError(t, err)
	assert.Equal(t, 2, count, "the orphan must not keep the badge lit")

	require.NoError(t, repo.CleanupExpired(ctx))
	var left int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE id = 'orphan'`).Scan(&left))
	assert.Equal(t, 0, left, "the sweep removes the orphan for good")

	// The removal event path: the title's notifications go at once, the rest stay.
	require.NoError(t, repo.DeleteByMediaItem(ctx, "media-1"))
	list, err = repo.ListByProfile(ctx, "pO", false, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "no-media", list[0].ID)
}

func TestNotificationRepo_CRUD(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedNotifProfile(t, db, "profile-a", "notif-u1")

	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()

	expires := time.Now().Add(24 * time.Hour)
	n := &notification.Notification{
		ID:          "notif-1",
		ProfileID:   "profile-a",
		Type:        notification.TypeNewMovie,
		Title:       "New Movie: Some Movie",
		Message:     "Some Movie added to your library",
		MediaItemID: "media-1",
		Level:       notification.LevelAll,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
		ExpiresAt:   &expires,
	}

	require.NoError(t, repo.Create(ctx, n))

	list, err := repo.ListByProfile(ctx, "profile-a", false, 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, n.ID, list[0].ID)
	assert.Equal(t, notification.TypeNewMovie, list[0].Type)

	// The série grouping fields must survive the round-trip: a client that
	// words its own sentence reads Subject + the counts, never Title.
	grouped := &notification.Notification{
		ID: "notif-2", ProfileID: "profile-a", Type: notification.TypeNewEpisode,
		Title: "New Episodes: Some Series", MediaItemID: "media-1",
		SeriesKey: notification.NewSeriesKey("lib1", "Some Series"),
		Subject:   "Some Series", SeasonCount: 2, EpisodeCount: 16,
		Level: notification.LevelAll, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, grouped))
	list, err = repo.ListByProfile(ctx, "profile-a", false, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 2)
	var got *notification.Notification
	for _, item := range list {
		if item.ID == "notif-2" {
			got = item
		}
	}
	require.NotNil(t, got)
	assert.Equal(t, "Some Series", got.Subject)
	assert.Equal(t, 2, got.SeasonCount)
	assert.Equal(t, 16, got.EpisodeCount)
	assert.Equal(t, notification.NewSeriesKey("lib1", "Some Series"), got.SeriesKey)
	require.NoError(t, repo.MarkRead(ctx, "notif-2", "profile-a"))

	count, err := repo.UnreadCount(ctx, "profile-a")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	require.NoError(t, repo.MarkRead(ctx, n.ID, "profile-a"))
	count, err = repo.UnreadCount(ctx, "profile-a")
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// unread only filter
	unread, err := repo.ListByProfile(ctx, "profile-a", true, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, unread)

	require.NoError(t, repo.DeleteAll(ctx, "profile-a"))
	list, _ = repo.ListByProfile(ctx, "profile-a", false, 10, 0)
	assert.Empty(t, list)
}

func TestNotificationRepo_MarkAllRead(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedNotifProfile(t, db, "pA", "notif-u2")

	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()

	for i, id := range []string{"n1", "n2", "n3"} {
		require.NoError(t, repo.Create(ctx, &notification.Notification{
			ID: id, ProfileID: "pA",
			Type: notification.TypeNewMovie, Title: "T", Level: notification.LevelAll,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}))
	}

	require.NoError(t, repo.MarkAllRead(ctx, "pA"))
	count, err := repo.UnreadCount(ctx, "pA")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestNotificationRepo_CleanupExpired(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedNotifProfile(t, db, "pB", "notif-u3")

	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	require.NoError(t, repo.Create(ctx, &notification.Notification{
		ID: "expired", ProfileID: "pB",
		Type: notification.TypeNewMovie, Title: "Old", Level: notification.LevelAll,
		CreatedAt: time.Now(), ExpiresAt: &past,
	}))
	require.NoError(t, repo.Create(ctx, &notification.Notification{
		ID: "active", ProfileID: "pB",
		Type: notification.TypeNewMovie, Title: "New", Level: notification.LevelAll,
		CreatedAt: time.Now(), ExpiresAt: &future,
	}))

	require.NoError(t, repo.CleanupExpired(ctx))

	list, err := repo.ListByProfile(ctx, "pB", false, 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "active", list[0].ID)
}

func TestNotificationRepo_FindRecentByProfilesTypeAndSeries(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedNotifProfile(t, db, "pBatchA", "notif-batch-u1")
	seedNotifProfile(t, db, "pBatchB", "notif-batch-u2")

	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expires := now.Add(24 * time.Hour)

	mk := func(id, profileID string, typ notification.Type, relatedMediaID string, createdAt time.Time) {
		require.NoError(t, repo.Create(ctx, &notification.Notification{
			ID: id, ProfileID: shared.ProfileID(profileID), Type: typ, Title: "T",
			SeriesKey: notification.SeriesKey(relatedMediaID),
			Level:     notification.LevelAll,
			CreatedAt: createdAt,
			ExpiresAt: &expires,
		}))
	}

	// Matches for pBatchA: a recent series notif for "lib1:Foo".
	mk("a-series", "pBatchA", notification.TypeNewSeries, "lib1:Foo", now)
	// Matches for pBatchB: a recent episode notif for the same series key.
	mk("b-episode", "pBatchB", notification.TypeNewEpisode, "lib1:Foo", now)
	// Must NOT match: different series key.
	mk("a-other-series", "pBatchA", notification.TypeNewMovie, "lib1:Bar", now)
	// Must NOT match: outside the since window.
	mk("a-old", "pBatchA", notification.TypeNewSeries, "lib1:Foo", now.Add(-48*time.Hour))
	// Must NOT match: profile not in the requested set.
	seedNotifProfile(t, db, "pBatchC", "notif-batch-u3")
	mk("c-series", "pBatchC", notification.TypeNewSeries, "lib1:Foo", now)

	since := now.Add(-24 * time.Hour)
	types := []notification.Type{notification.TypeNewSeries, notification.TypeNewSeason, notification.TypeNewEpisode}

	byProfile, err := repo.FindRecentByProfilesTypeAndSeries(ctx, []shared.ProfileID{"pBatchA", "pBatchB"}, types, "lib1:Foo", since)
	require.NoError(t, err)

	require.Len(t, byProfile["pBatchA"], 1)
	assert.Equal(t, "a-series", byProfile["pBatchA"][0].ID)

	require.Len(t, byProfile["pBatchB"], 1)
	assert.Equal(t, "b-episode", byProfile["pBatchB"][0].ID)

	assert.Empty(t, byProfile["pBatchC"], "profile outside the requested set must not appear in the result")

	// Empty profile list short-circuits without querying.
	empty, err := repo.FindRecentByProfilesTypeAndSeries(ctx, nil, types, "lib1:Foo", since)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestNotificationSettingsRepo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewNotificationSettingsRepository(db)
	ctx := context.Background()

	// global defaults should return all=all initially
	defaults, err := repo.GetGlobalDefaults(ctx)
	require.NoError(t, err)
	assert.Equal(t, notification.LevelAll, defaults.MovieLevel)

	// update global defaults
	require.NoError(t, repo.UpsertGlobalDefaults(ctx, &notification.Settings{
		MovieLevel:  notification.LevelRelevant,
		SeriesLevel: notification.LevelAll,
		MusicLevel:  notification.LevelDisabled,
	}))

	defaults, err = repo.GetGlobalDefaults(ctx)
	require.NoError(t, err)
	assert.Equal(t, notification.LevelRelevant, defaults.MovieLevel)
	assert.Equal(t, notification.LevelDisabled, defaults.MusicLevel)

	// profile settings - falls back to global if not set
	profSettings, err := repo.GetByProfile(ctx, "pX")
	require.NoError(t, err)
	assert.Equal(t, notification.LevelRelevant, profSettings.MovieLevel)

	// upsert profile-specific
	require.NoError(t, repo.Upsert(ctx, &notification.Settings{
		ProfileID:   "pX",
		MovieLevel:  notification.LevelAll,
		SeriesLevel: notification.LevelHighlyRelevant,
		MusicLevel:  notification.LevelAll,
	}))
	profSettings, err = repo.GetByProfile(ctx, "pX")
	require.NoError(t, err)
	assert.Equal(t, notification.LevelAll, profSettings.MovieLevel)
	assert.Equal(t, notification.LevelHighlyRelevant, profSettings.SeriesLevel)
}

// MarkRead used to answer nil whatever happened: its WHERE clause carried both
// the ownership guard and a `read_at IS NULL` filter, so zero rows meant either
// "already read" or "not yours / no such id", and the result was dropped. A
// client could mark another profile's notification and be told it worked.
func TestNotificationRepo_MarkReadDistinguishesNotFoundFromAlreadyRead(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedNotifProfile(t, db, "profile-a", "notif-mark-u1")
	seedNotifProfile(t, db, "profile-b", "notif-mark-u2")

	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()

	n := &notification.Notification{
		ID:        "notif-mark-1",
		ProfileID: "profile-a",
		Type:      notification.TypeNewMovie,
		Title:     "New Movie: Some Movie",
		CreatedAt: time.Now(),
	}
	require.NoError(t, repo.Create(ctx, n))

	// First mark: the real one.
	require.NoError(t, repo.MarkRead(ctx, n.ID, "profile-a"))

	// Marking it again must stay a no-op, not an error: clients retry.
	require.NoError(t, repo.MarkRead(ctx, n.ID, "profile-a"), "re-marking a read notification must remain idempotent")

	// An id that does not exist is the caller's mistake and must say so.
	require.ErrorIs(t, repo.MarkRead(ctx, "notif-does-not-exist", "profile-a"), shared.ErrNotFound)

	// Someone else's notification answers exactly the same way, so the id
	// stays unguessable - and it must NOT have been marked read.
	require.ErrorIs(t, repo.MarkRead(ctx, n.ID, "profile-b"), shared.ErrNotFound)
}

// Batching rewrites the grouped fields of an existing row (subject, season
// and episode counts, title) - scoped to the owning profile, so another
// profile's row with the same id is never touched.
func TestNotificationRepo_Update_RewritesGroupedFieldsForOwnerOnly(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedNotifProfile(t, db, "profile-up", "notif-up")
	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()

	n := &notification.Notification{
		ID: "notif-up-1", ProfileID: "profile-up", Type: notification.TypeNewEpisode,
		Title: "New Episodes: Some Series", Subject: "Some Series", SeasonCount: 1, EpisodeCount: 1,
		Level: notification.LevelAll, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, n))

	n.Title, n.SeasonCount, n.EpisodeCount = "New Episodes: Some Series (3)", 2, 3
	require.NoError(t, repo.Update(ctx, n))
	list, err := repo.ListByProfile(ctx, "profile-up", false, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "New Episodes: Some Series (3)", list[0].Title)
	assert.Equal(t, 2, list[0].SeasonCount)
	assert.Equal(t, 3, list[0].EpisodeCount)

	// Another profile cannot rewrite it through the same id.
	foreign := *n
	foreign.ProfileID = "profile-other"
	foreign.Title = "hijacked"
	require.NoError(t, repo.Update(ctx, &foreign))
	list, _ = repo.ListByProfile(ctx, "profile-up", false, 10, 0)
	assert.Equal(t, "New Episodes: Some Series (3)", list[0].Title)
}
