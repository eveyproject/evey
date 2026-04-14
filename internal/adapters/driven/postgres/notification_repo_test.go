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

func seedNotifProfile(t *testing.T, db *sql.DB, profileID, userID string) {
	t.Helper()
	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	require.NoError(t, userRepo.Create(ctx, newTestUser(userID)))
	require.NoError(t, profileRepo.Create(ctx, newTestProfile(profileID, shared.UserID(userID))))
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
		Title:       "New Movie: Dune",
		Message:     "Dune added to your library",
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
