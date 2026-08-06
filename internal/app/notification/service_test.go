package notification_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appnotification "github.com/evey-project/evey/internal/app/notification"
	"github.com/evey-project/evey/internal/domain/backup"
	libevent "github.com/evey-project/evey/internal/domain/library"
	notifdom "github.com/evey-project/evey/internal/domain/notification"
	notiffakes "github.com/evey-project/evey/internal/domain/notification/fakes"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
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

	require.NoError(t, cmdSvc.MarkRead(ctx, "n1", "p1"))
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

	// Episode with SeriesTitle is batched - flush to emit notification
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
	_ = repo.MarkRead(ctx, "n1", "p1")

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
	appnotification.WithPush(pushSender, subRepo)(svc)
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
	appnotification.WithPush(pushSender, subRepo)(svc)
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
	appnotification.WithPush(pushSender, subRepo)(svc)
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
	appnotification.WithPush(pushSender, subRepo)(svc)
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

func TestNotificationService_PluginError_NotifiesAllProfilesWhenNoAdminLister(t *testing.T) {
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

	bus.Publish(ctx, plugin.PluginError{
		PluginID:     "themer",
		ErrorMessage: "sandbox unavailable",
		OccurredAt:   time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypePluginError, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "themer")
	assert.Equal(t, "sandbox unavailable", notifs[0].Message)
}

type fakeAdminLister struct {
	profiles []*user.Profile
}

func (f *fakeAdminLister) ListAdminProfiles(_ context.Context) ([]*user.Profile, error) {
	return f.profiles, nil
}

func TestNotificationService_PluginError_RoutesAdminOnlyWithAdminLister(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	adminProfile := &user.Profile{ID: "p-admin", UserID: "u-admin", Name: "Root"}
	userProfile := &user.Profile{ID: "p-user", UserID: "u-user", Name: "Bob"}
	_ = profileRepo.Create(ctx, adminProfile)
	_ = profileRepo.Create(ctx, userProfile)

	admin := &fakeAdminLister{profiles: []*user.Profile{adminProfile}}
	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus,
		appnotification.WithAdminLister(admin))
	svc.Register()

	bus.Publish(ctx, plugin.PluginError{
		PluginID:     "themer",
		ErrorMessage: "crashed",
		OccurredAt:   time.Now(),
	})

	adminNotifs, err := repo.ListByProfile(ctx, "p-admin", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, adminNotifs, 1, "admin profile should receive plugin.error")

	userNotifs, err := repo.ListByProfile(ctx, "p-user", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, userNotifs, "non-admin profile must NOT receive plugin.error")
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
	appnotification.WithPush(pushSender, subRepo)(svc)
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

	// Assert it, don't infer it. "Tested implicitly by the fact that the handler
	// only processes backup.BackupFailed" is a claim about the code, not a test:
	// if the backup handler DID start reacting to MediaItemAdded, this test
	// stayed green. Only a backup notification would be wrong here - the
	// media-added handler legitimately produces its own.
	got, err := repo.ListByProfile(ctx, "p1", false, 100, 0)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	for _, n := range got {
		if strings.Contains(strings.ToLower(string(n.Type)), "backup") {
			t.Fatalf("the backup handler must ignore MediaItemAdded, got a %q notification", n.Type)
		}
	}
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
	initDone := make(chan struct{})
	cmdSvc.StartCleanup(ctx2, initDone)
	<-initDone
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
			MediaItemID: shared.MediaItemID(fmt.Sprintf("sn%d", i)), LibraryID: "lib1",
			Title: fmt.Sprintf("Season %d", i+1), SeriesTitle: "Breaking Bad",
			Type: "season", OccurredAt: time.Now(),
		})
	}
	for i := range 5 {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: shared.MediaItemID(fmt.Sprintf("ep%d", i)), LibraryID: "lib1",
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
			MediaItemID: shared.MediaItemID(fmt.Sprintf("ep%d", i)), LibraryID: "lib1",
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

func TestNotificationService_Batch_SeasonsOnly_GroupedAndNamed(t *testing.T) {
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

	// 3 seasons of an existing series added (no series event, no episodes):
	// must collapse to a single notification that names the series.
	for i := range 3 {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: shared.MediaItemID(fmt.Sprintf("sn%d", i)), LibraryID: "lib1",
			Title: fmt.Sprintf("Season %d", i+1), SeriesTitle: "The Sopranos",
			Type: "season", OccurredAt: time.Now(),
		})
	}

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "multiple seasons should group into 1 notification")
	assert.Equal(t, notifdom.TypeNewSeason, notifs[0].Type)
	assert.Contains(t, notifs[0].Title, "The Sopranos")
	assert.Contains(t, notifs[0].Title, "3 seasons")
}

func TestNotificationService_Batch_SingleSeason_NamesSeries(t *testing.T) {
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
		MediaItemID: "sn1", LibraryID: "lib1",
		Title: "Season 4", SeriesTitle: "The Sopranos",
		Type: "season", OccurredAt: time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewSeason, notifs[0].Type)
	assert.Equal(t, "new_season:The Sopranos - Season 4", notifs[0].Title)
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

	// Movie should be immediate - no flush needed
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
			MediaItemID: shared.MediaItemID(fmt.Sprintf("ep%d", i)), LibraryID: "lib1",
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
	appnotification.WithBatchDelay(50 * time.Millisecond)(svc)
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

// TestNoEpisodeNotificationWhenSeriesNotificationCreatedRecently verifies that
// when a new_series notification was already emitted for a series within 24h,
// subsequent new_episode notifications for the same series are suppressed.
func TestNoEpisodeNotificationWhenSeriesNotificationCreatedRecently(t *testing.T) {
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

	// First batch: series + season + episodes - produces exactly one new_series notif.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "sr1", LibraryID: "lib1",
		Title: "Stranger Things", SeriesTitle: "Stranger Things",
		Type: "series", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1", LibraryID: "lib1",
		Title: "Chapter One", SeriesTitle: "Stranger Things",
		Type: "episode", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep2", LibraryID: "lib1",
		Title: "Chapter Two", SeriesTitle: "Stranger Things",
		Type: "episode", OccurredAt: time.Now(),
	})

	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "first import: only new_series expected")
	assert.Equal(t, notifdom.TypeNewSeries, notifs[0].Type)

	// Second batch: more episodes from the same series arrive (dedupe key is new,
	// different episode IDs). Should be suppressed because new_series was recent.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep3", LibraryID: "lib1",
		Title: "Chapter Three", SeriesTitle: "Stranger Things",
		Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err = repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "episode after recent new_series must be suppressed")
}

// TestNoEpisodeNotificationWhenSeasonNotificationCreatedRecently verifies that
// a new_season notification for a series suppresses subsequent new_episode
// notifications within the 24h window.
func TestNoEpisodeNotificationWhenSeasonNotificationCreatedRecently(t *testing.T) {
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

	// First batch: season + episodes → new_season notif.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "sn1", LibraryID: "lib1",
		Title: "Season 3", SeriesTitle: "Breaking Bad",
		Type: "season", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1", LibraryID: "lib1",
		Title: "S03E01", SeriesTitle: "Breaking Bad",
		Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, notifdom.TypeNewSeason, notifs[0].Type)

	// Subsequent episode from same series should be suppressed.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep2", LibraryID: "lib1",
		Title: "S03E02", SeriesTitle: "Breaking Bad",
		Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err = repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "episode after recent new_season must be suppressed")
}

// TestEpisodeNotificationsAreGroupedWithin24h verifies that when a new_episode
// notification already exists for a series within 24h, a subsequent episode
// import updates that notification rather than creating a new one.
func TestEpisodeNotificationsAreGroupedWithin24h(t *testing.T) {
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

	// First episode for a known series.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep7", LibraryID: "lib1",
		Title: "S01E07", SeriesTitle: "The Wire",
		Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "first episode should create one notification")
	assert.Equal(t, notifdom.TypeNewEpisode, notifs[0].Type)

	// Second episode within 24h - should update the existing notification.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep8", LibraryID: "lib1",
		Title: "S01E08", SeriesTitle: "The Wire",
		Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err = repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "second episode within 24h should update existing notification, not create a new one")
	assert.Contains(t, notifs[0].Title, "The Wire", "updated title should contain series name")
	assert.Contains(t, notifs[0].Title, "2 episodes", "updated title should show accumulated count")

	// Third episode within 24h - count should now be 3.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep9", LibraryID: "lib1",
		Title: "S01E09", SeriesTitle: "The Wire",
		Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err = repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "third episode within 24h must still result in exactly 1 notification")
	assert.Contains(t, notifs[0].Title, "3 episodes")
}

// TestEpisodeNotificationCreatedAfter24hWindow verifies that when the grouping
// window has elapsed, a new independent notification is created instead of
// updating the older one.
func TestEpisodeNotificationCreatedAfter24hWindow(t *testing.T) {
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

	// Directly inject an old new_episode notification that is 25h old (beyond window).
	seriesKey := "lib1:Homeland"
	oldCreatedAt := time.Now().Add(-25 * time.Hour)
	expires := time.Now().AddDate(0, 0, 30)
	_ = repo.Create(ctx, &notifdom.Notification{
		ID:             "old-ep",
		ProfileID:      "p1",
		Type:           notifdom.TypeNewEpisode,
		Title:          "new_episode:Homeland - S02E05",
		SeriesKey: notifdom.SeriesKey(seriesKey),
		EpisodeCount:   1,
		Level:          notifdom.LevelAll,
		CreatedAt:      oldCreatedAt,
		ExpiresAt:      &expires,
	})

	// Now a new episode arrives - the old notif is outside the 24h window.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep6", LibraryID: "lib1",
		Title: "S02E06", SeriesTitle: "Homeland",
		Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 2, "after 24h window a new separate notification should be created")
}
