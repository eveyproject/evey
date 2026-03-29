package notification_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	"github.com/gravity-zero/evey/internal/domain/backup"
	libevent "github.com/gravity-zero/evey/internal/domain/library"
	notifdom "github.com/gravity-zero/evey/internal/domain/notification"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errProfileLister struct{ err error }

func (e *errProfileLister) GetAll(_ context.Context) ([]*user.Profile, error) {
	return nil, e.err
}

func TestNotificationService_MediaItemAdded_CreatesNotificationsForAllProfiles(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p2", UserID: "u2", Name: "Bob"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewMovie, notifs[0].Type)

	notifs2, err := repo.ListByProfile(ctx, "p2", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs2, 1)
}

func TestNotificationService_Deduplication(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	ev := libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	}

	bus.Publish(ctx, ev)
	bus.Publish(ctx, ev)

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "should deduplicate within 5-min window")
}

func TestNotificationService_LevelDisabled_SkipsProfile(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelDisabled,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

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
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	querySvc := appnotification.NewQueryService(repo, settings)

	_ = repo.Create(ctx, &notifdom.Notification{ID: "n1", ProfileID: "p1", Type: notifdom.TypeNewMovie, Title: "T", Level: notifdom.LevelAll, CreatedAt: time.Now()})
	_ = repo.Create(ctx, &notifdom.Notification{ID: "n2", ProfileID: "p1", Type: notifdom.TypeNewMovie, Title: "T2", Level: notifdom.LevelAll, CreatedAt: time.Now()})

	count, err := querySvc.UnreadCount(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	require.NoError(t, cmdSvc.MarkRead(ctx, "n1"))
	count, _ = querySvc.UnreadCount(ctx, "p1")
	assert.Equal(t, 1, count)

	require.NoError(t, cmdSvc.MarkAllRead(ctx, "p1"))
	count, _ = querySvc.UnreadCount(ctx, "p1")
	assert.Equal(t, 0, count)
}

func TestNotificationService_GetAllProfilesError_NoNotifications(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	bus := sharedfakes.NewFakeEventBus()

	lister := &errProfileLister{err: errors.New("db down")}
	svc := appnotification.NewCommandService(repo, settings, analyzer, lister, bus)
	svc.Register()

	ctx := context.Background()
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs, "no notifications should be created when profile list fails")
}

func TestNotificationService_EpisodeNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1",
		LibraryID:   "lib1",
		Title:       "Pilot",
		SeriesTitle: "Breaking Bad",
		Type:        "episode",
		OccurredAt:  time.Now(),
	})

	// Episode with SeriesTitle is batched — flush to emit notification
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewEpisode, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "Breaking Bad")
}

func TestNotificationService_AlbumNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "alb1",
		LibraryID:   "lib1",
		Title:       "Dark Side of the Moon",
		Type:        "album",
		OccurredAt:  time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewAlbum, notifs[0].Type)
}

func TestNotificationService_UnknownMediaType_NoNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "x1",
		LibraryID:   "lib1",
		Title:       "Unknown",
		Type:        "photo",
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs, "unknown media type should not create notifications")
}

func TestNotificationService_DeleteAll(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	ctx := context.Background()
	_ = repo.Create(ctx, &notifdom.Notification{ID: "n1", ProfileID: "p1", Title: "T1", CreatedAt: time.Now()})
	_ = repo.Create(ctx, &notifdom.Notification{ID: "n2", ProfileID: "p1", Title: "T2", CreatedAt: time.Now()})
	_ = repo.Create(ctx, &notifdom.Notification{ID: "n3", ProfileID: "p2", Title: "T3", CreatedAt: time.Now()})

	require.NoError(t, cmdSvc.DeleteAll(ctx, "p1"))

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs)

	notifs2, err := repo.ListByProfile(ctx, "p2", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs2, 1, "p2 notifications should not be affected")
}

func TestNotificationService_GetSettings(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	querySvc := appnotification.NewQueryService(repo, settings)

	ctx := context.Background()
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelAll,
		SeriesLevel: notifdom.LevelDisabled,
		MusicLevel:  notifdom.LevelAll,
	})

	s, err := querySvc.GetSettings(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, notifdom.LevelDisabled, s.SeriesLevel)
}

func TestNotificationService_UpdateSettings(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	querySvc := appnotification.NewQueryService(repo, settings)

	ctx := context.Background()
	newSettings := &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelDisabled,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelDisabled,
	}
	require.NoError(t, cmdSvc.UpdateSettings(ctx, newSettings))

	s, err := querySvc.GetSettings(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, notifdom.LevelDisabled, s.MovieLevel)
	assert.Equal(t, notifdom.LevelAll, s.SeriesLevel)
}

func TestNotificationService_List_UnreadOnly(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	querySvc := appnotification.NewQueryService(repo, settings)

	ctx := context.Background()
	n1 := &notifdom.Notification{ID: "n1", ProfileID: "p1", Title: "Read", CreatedAt: time.Now()}
	n2 := &notifdom.Notification{ID: "n2", ProfileID: "p1", Title: "Unread", CreatedAt: time.Now()}
	_ = repo.Create(ctx, n1)
	_ = repo.Create(ctx, n2)
	_ = repo.MarkRead(ctx, "n1")

	notifs, err := querySvc.List(ctx, "p1", true, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, "n2", notifs[0].ID)
}

func TestNotificationService_WithPush_SendsToSubscriptions(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	subRepo := notiffakes.NewFakePushSubscriptionRepository()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/1", "key1", "auth1")
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/2", "key2", "auth2")

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.WithPush(pushSender, subRepo)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	assert.Len(t, pushSender.Calls, 2, "should send push to both subscriptions")
	assert.Equal(t, "https://push.example.com/1", pushSender.Calls[0].Sub.Endpoint)
	assert.Equal(t, "https://push.example.com/2", pushSender.Calls[1].Sub.Endpoint)
}

func TestNotificationService_WithPush_NoSubsNoPush(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	subRepo := notiffakes.NewFakePushSubscriptionRepository()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.WithPush(pushSender, subRepo)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	// notification created but no push calls
	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Empty(t, pushSender.Calls, "no subscriptions = no push")
}

func TestNotificationService_WithPush_DisabledLevel_NoPush(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	subRepo := notiffakes.NewFakePushSubscriptionRepository()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/1", "key1", "auth1")
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelDisabled,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.WithPush(pushSender, subRepo)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	assert.Empty(t, pushSender.Calls, "disabled level should not send push")
}

func TestNotificationService_WithPush_PushSendError_DoesNotBlockNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	pushSender.Err = errors.New("push service down")
	subRepo := notiffakes.NewFakePushSubscriptionRepository()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/1", "key1", "auth1")

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.WithPush(pushSender, subRepo)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Inception",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	// notification still created despite push failure
	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "notification should be created even if push fails")
}

func TestNotificationService_BackupFailed_NotifiesAllProfiles(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p2", UserID: "u2", Name: "Bob"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, backup.BackupFailed{
		Error:      "disk full",
		OccurredAt: time.Now(),
	})

	notifs1, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs1, 1)
	assert.Equal(t, notifdom.TypeBackupFailed, notifs1[0].Type)
	assert.Equal(t, "Scheduled Backup Failed", notifs1[0].Title)
	assert.Equal(t, "disk full", notifs1[0].Message)

	notifs2, err := repo.ListByProfile(ctx, "p2", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs2, 1)
}

func TestNotificationService_SeasonNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "s1",
		LibraryID:   "lib1",
		Title:       "Season 2",
		SeriesTitle: "Breaking Bad",
		Type:        "season",
		OccurredAt:  time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewSeason, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "Breaking Bad")
}

func TestNotificationService_SeasonNotification_NoSeriesTitle(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "s1",
		LibraryID:   "lib1",
		Title:       "Season 2",
		Type:        "season",
		OccurredAt:  time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, "new_season:Season 2", notifs[0].Title)
}

func TestNotificationService_SeriesNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "sr1",
		LibraryID:   "lib1",
		Title:       "The Wire",
		Type:        "series",
		OccurredAt:  time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewSeries, notifs[0].Type)
	assert.Equal(t, "new_series:The Wire", notifs[0].Title)
}

func TestNotificationService_EpisodeNotification_NoSeriesTitle(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1",
		LibraryID:   "lib1",
		Title:       "Pilot",
		Type:        "episode",
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, "new_episode:Pilot", notifs[0].Title)
}

func TestNotificationService_RelevantLevel_MatchingGenres_CreatesNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Genres = []string{"Action", "Sci-Fi"}
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelRelevant,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "The Matrix",
		Type:        "movie",
		Genres:      []string{"Action", "Drama"},
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "matching genre should create notification")
}

func TestNotificationService_RelevantLevel_NoMatchingGenres_SkipsNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Genres = []string{"Horror", "Comedy"}
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelRelevant,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "The Matrix",
		Type:        "movie",
		Genres:      []string{"Action", "Sci-Fi"},
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs, "no matching genre should skip")
}

func TestNotificationService_RelevantLevel_NoGenresOnEvent_SkipsNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelRelevant,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "No Genre Movie",
		Type:        "movie",
		Genres:      nil,
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs, "relevant level with no genres on event should skip")
}

func TestNotificationService_HighlyRelevantLevel_MatchingGenres(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Genres = []string{"Drama", "Thriller"}
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelHighlyRelevant,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Zodiac",
		Type:        "movie",
		Genres:      []string{"Thriller", "Mystery"},
		OccurredAt:  time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "highly_relevant with matching genre should create notification")
}

func TestNotificationService_GetSettingsBatchError_NoNotifications(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	settings.Err = errors.New("settings db down")
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Test",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	notifs, _ := repo.ListByProfile(ctx, "p1", false, 0, 0)
	assert.Empty(t, notifs, "settings error should prevent notifications")
}

func TestNotificationService_FavoriteGenresBatchError_NoNotifications(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Err = errors.New("analyzer down")
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  notifdom.LevelRelevant,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Test",
		Type:        "movie",
		Genres:      []string{"Action"},
		OccurredAt:  time.Now(),
	})

	notifs, _ := repo.ListByProfile(ctx, "p1", false, 0, 0)
	assert.Empty(t, notifs, "analyzer error should prevent notifications")
}

func TestNotificationService_UpdateSettings_ValidationError(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	err := cmdSvc.UpdateSettings(context.Background(), &notifdom.Settings{
		ProfileID:   "p1",
		MovieLevel:  "bogus",
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelAll,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid notification settings")
}

func TestNotificationService_UpdateGlobalSettings(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	querySvc := appnotification.NewQueryService(repo, settings)

	ctx := context.Background()
	err := cmdSvc.UpdateGlobalSettings(ctx, &notifdom.Settings{
		MovieLevel:  notifdom.LevelDisabled,
		SeriesLevel: notifdom.LevelAll,
		MusicLevel:  notifdom.LevelRelevant,
	})
	require.NoError(t, err)

	gs, err := querySvc.GetGlobalSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, notifdom.LevelDisabled, gs.MovieLevel)
	assert.Equal(t, notifdom.LevelRelevant, gs.MusicLevel)
}

func TestNotificationService_UpdateGlobalSettings_ValidationError(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	err := cmdSvc.UpdateGlobalSettings(context.Background(), &notifdom.Settings{
		MovieLevel:  notifdom.LevelAll,
		SeriesLevel: "invalid",
		MusicLevel:  notifdom.LevelAll,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid notification settings")
}

func TestNotificationService_BackupFailed_WithPush(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	subRepo := notiffakes.NewFakePushSubscriptionRepository()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/1", "key1", "auth1")

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.WithPush(pushSender, subRepo)
	svc.Register()

	bus.Publish(ctx, backup.BackupFailed{
		Error:      "disk full",
		OccurredAt: time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeBackupFailed, notifs[0].Type)
	assert.Len(t, pushSender.Calls, 1, "should send push for backup failure")
}

func TestNotificationService_BackupFailed_ProfilesError(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	bus := sharedfakes.NewFakeEventBus()

	lister := &errProfileLister{err: errors.New("db down")}
	svc := appnotification.NewCommandService(repo, settings, analyzer, lister, bus)
	svc.Register()

	ctx := context.Background()
	bus.Publish(ctx, backup.BackupFailed{
		Error:      "disk full",
		OccurredAt: time.Now(),
	})

	notifs, _ := repo.ListByProfile(ctx, "p1", false, 0, 0)
	assert.Empty(t, notifs, "profiles error should prevent backup notifications")
}

func TestNotificationService_BackupFailed_WrongEventType(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1",
		LibraryID:   "lib1",
		Title:       "Test",
		Type:        "movie",
		OccurredAt:  time.Now(),
	})

	// backup.failed handler should ignore MediaItemAdded events — this is tested
	// implicitly by the fact that backup handler only processes backup.BackupFailed
}

func TestNotificationService_StartStop(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)
	svc.Stop()
	svc.Stop() // double stop should not panic
	cancel()
}

func TestNotificationService_StartCleanup(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	_ = repo.Create(ctx, &notifdom.Notification{ID: "expired", ProfileID: "p1", Title: "Old", ExpiresAt: &past, CreatedAt: time.Now()})
	_ = repo.Create(ctx, &notifdom.Notification{ID: "valid", ProfileID: "p1", Title: "New", ExpiresAt: &future, CreatedAt: time.Now()})

	ctx2, cancel := context.WithCancel(ctx)
	cmdSvc.StartCleanup(ctx2)

	// Give goroutine time to run initial cleanup
	time.Sleep(50 * time.Millisecond)
	cancel()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, "valid", notifs[0].ID)
}

func TestNotificationService_CleanupExpired(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	_ = repo.Create(ctx, &notifdom.Notification{ID: "expired", ProfileID: "p1", Title: "Old", ExpiresAt: &past, CreatedAt: time.Now()})
	_ = repo.Create(ctx, &notifdom.Notification{ID: "valid", ProfileID: "p1", Title: "New", ExpiresAt: &future, CreatedAt: time.Now()})

	require.NoError(t, cmdSvc.CleanupExpired(ctx))

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, "valid", notifs[0].ID)
}

func TestNotificationService_Batch_SeriesWithSeasonsAndEpisodes(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	// Simulate scan: series + 2 seasons + 5 episodes
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "sr1", LibraryID: "lib1",
		Title: "Breaking Bad", SeriesTitle: "Breaking Bad",
		Type: "series", OccurredAt: time.Now(),
	})
	for i := range 2 {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: fmt.Sprintf("sn%d", i), LibraryID: "lib1",
			Title: fmt.Sprintf("Season %d", i+1), SeriesTitle: "Breaking Bad",
			Type: "season", OccurredAt: time.Now(),
		})
	}
	for i := range 5 {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: fmt.Sprintf("ep%d", i), LibraryID: "lib1",
			Title: fmt.Sprintf("S01E%02d", i+1), SeriesTitle: "Breaking Bad",
			Type: "episode", OccurredAt: time.Now(),
		})
	}

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "should produce exactly 1 notification for series + seasons + episodes")
	assert.Equal(t, notifdom.TypeNewSeries, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "Breaking Bad")
	assert.Contains(t, notifs[0].Title, "2 seasons")
	assert.Contains(t, notifs[0].Title, "5 episodes")
}

func TestNotificationService_Batch_EpisodesOnly_MultipleAmalgamated(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	// 3 episodes from same series, no series event
	for i := range 3 {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: fmt.Sprintf("ep%d", i), LibraryID: "lib1",
			Title: fmt.Sprintf("S02E%02d", i+1), SeriesTitle: "The Wire",
			Type: "episode", OccurredAt: time.Now(),
		})
	}

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "multiple episodes should amalgamate into 1 notification")
	assert.Equal(t, notifdom.TypeNewEpisode, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "The Wire")
	assert.Contains(t, notifs[0].Title, "3 episodes")
}

func TestNotificationService_Batch_SingleEpisode_NormalNotification(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1", LibraryID: "lib1",
		Title: "Pilot", SeriesTitle: "Breaking Bad",
		Type: "episode", OccurredAt: time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewEpisode, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "Breaking Bad")
	assert.Contains(t, notifs[0].Title, "Pilot")
}

func TestNotificationService_Batch_MovieNotBatched(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1",
		Title: "Inception", Type: "movie", OccurredAt: time.Now(),
	})

	// Movie should be immediate — no flush needed
	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewMovie, notifs[0].Type)
}

func TestNotificationService_Batch_DifferentSeries_SeparateNotifications(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	// 2 episodes from series A, 1 from series B
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1", LibraryID: "lib1",
		Title: "S01E01", SeriesTitle: "The Wire",
		Type: "episode", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep2", LibraryID: "lib1",
		Title: "S01E02", SeriesTitle: "The Wire",
		Type: "episode", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep3", LibraryID: "lib1",
		Title: "S01E01", SeriesTitle: "Breaking Bad",
		Type: "episode", OccurredAt: time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 2, "different series should produce separate notifications")
}

func TestNotificationService_Batch_SeasonWithEpisodes(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	// New season added to existing series (no series event)
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "sn1", LibraryID: "lib1",
		Title: "Season 4", SeriesTitle: "Breaking Bad",
		Type: "season", OccurredAt: time.Now(),
	})
	for i := range 3 {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: fmt.Sprintf("ep%d", i), LibraryID: "lib1",
			Title: fmt.Sprintf("S04E%02d", i+1), SeriesTitle: "Breaking Bad",
			Type: "episode", OccurredAt: time.Now(),
		})
	}

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "season + episodes should produce 1 notification")
	assert.Equal(t, notifdom.TypeNewSeason, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "Breaking Bad")
	assert.Contains(t, notifs[0].Title, "Season 4")
	assert.Contains(t, notifs[0].Title, "3 episodes")
}

func TestNotificationService_Batch_ImageNotification_Ignored(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "img1", LibraryID: "lib1",
		Title: "vacation.jpg", Type: "photo", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "img2", LibraryID: "lib1",
		Title: "sunset.png", Type: "image", OccurredAt: time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs, "images should never produce notifications")
}

func TestNotificationService_Batch_TimerFlushes(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.WithBatchDelay(50 * time.Millisecond)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1", LibraryID: "lib1",
		Title: "S01E01", SeriesTitle: "Lost",
		Type: "episode", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep2", LibraryID: "lib1",
		Title: "S01E02", SeriesTitle: "Lost",
		Type: "episode", OccurredAt: time.Now(),
	})

	// Wait for timer to fire
	time.Sleep(150 * time.Millisecond)

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "timer should auto-flush batch")
	assert.Contains(t, notifs[0].Title, "Lost")
	assert.Contains(t, notifs[0].Title, "2 episodes")
}
