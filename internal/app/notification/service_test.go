package notification_test

import (
	"context"
	"testing"
	"time"

	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	libevent "github.com/gravity-zero/evey/internal/domain/library"
	notifdom "github.com/gravity-zero/evey/internal/domain/notification"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationService_MediaItemAdded_CreatesNotificationsForAllProfiles(t *testing.T) {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p2", UserID: "u2", Name: "Bob"})

	_ = appnotification.NewService(repo, settings, analyzer, profileRepo, bus)

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	// give goroutines time to process (event bus is sync in fake)
	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewMovie, notifs[0].Type)

	notifs2, err := repo.ListByProfile(ctx, "p2", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs2, 1)
}

func TestNotificationService_Deduplication(t *testing.T) {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	_ = appnotification.NewService(repo, settings, analyzer, profileRepo, bus)

	ev := libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	}

	bus.Publish(ctx, ev)
	bus.Publish(ctx, ev) // duplicate within 5 min window

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "should deduplicate within 5-min window")
}

func TestNotificationService_LevelDisabled_SkipsProfile(t *testing.T) {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	// disable movie notifications for p1
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelDisabled,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	_ = appnotification.NewService(repo, settings, analyzer, profileRepo, bus)

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs, "disabled level should skip")
}

func TestNotificationService_MarkRead_UnreadCount(t *testing.T) {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	svc := appnotification.NewService(repo, settings, analyzer, profileRepo, bus)

	_ = repo.Create(ctx, &notifdom.Notification{ID: "n1", ProfileID: "p1", Type: notifdom.TypeNewMovie, Title: "T", Level: notifdom.LevelAll, CreatedAt: time.Now()})
	_ = repo.Create(ctx, &notifdom.Notification{ID: "n2", ProfileID: "p1", Type: notifdom.TypeNewMovie, Title: "T2", Level: notifdom.LevelAll, CreatedAt: time.Now()})

	count, err := svc.UnreadCount(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	require.NoError(t, svc.MarkRead(ctx, "n1"))
	count, _ = svc.UnreadCount(ctx, "p1")
	assert.Equal(t, 1, count)

	require.NoError(t, svc.MarkAllRead(ctx, "p1"))
	count, _ = svc.UnreadCount(ctx, "p1")
	assert.Equal(t, 0, count)
}
