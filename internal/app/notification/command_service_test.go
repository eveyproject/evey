package notification_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	appnotification "github.com/evey-project/evey/internal/app/notification"
	libevent "github.com/evey-project/evey/internal/domain/library"
	notifdom "github.com/evey-project/evey/internal/domain/notification"
	notiffakes "github.com/evey-project/evey/internal/domain/notification/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCmdService(t *testing.T) (*appnotification.CommandService, *notiffakes.FakeNotificationRepository, *notiffakes.FakeSettingsRepository, *userfakes.FakeProfileRepository, *sharedfakes.FakeEventBus) {
	t.Helper()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	return svc, repo, settings, profileRepo, bus
}

func TestCommandService_MarkRead_Delegates(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, _ := newTestCmdService(t)
	ctx := context.Background()
	n := &notifdom.Notification{ID: "n1", ProfileID: shared.ProfileID("p1"), Type: notifdom.TypeNewMovie, CreatedAt: time.Now()}
	require.NoError(t, repo.Create(ctx, n))
	require.NoError(t, svc.MarkRead(ctx, "n1", "p1"))
	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.NotNil(t, notifs[0].ReadAt)
}

func TestCommandService_MarkRead_RepoError(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, _ := newTestCmdService(t)
	repo.Err = errors.New("boom")
	err := svc.MarkRead(context.Background(), "n1", "p1")
	assert.Error(t, err)
}

func TestCommandService_MarkAllRead_Delegates(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, _ := newTestCmdService(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &notifdom.Notification{ID: "n1", ProfileID: shared.ProfileID("p1"), CreatedAt: time.Now()}))
	require.NoError(t, repo.Create(ctx, &notifdom.Notification{ID: "n2", ProfileID: shared.ProfileID("p1"), CreatedAt: time.Now()}))
	require.NoError(t, svc.MarkAllRead(ctx, "p1"))
	cnt, err := repo.UnreadCount(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, 0, cnt)
}

func TestCommandService_DeleteAll_Delegates(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, _ := newTestCmdService(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &notifdom.Notification{ID: "n1", ProfileID: shared.ProfileID("p1"), CreatedAt: time.Now()}))
	require.NoError(t, svc.DeleteAll(ctx, "p1"))
	items, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestCommandService_UpdateSettings_Valid(t *testing.T) {
	t.Parallel()
	svc, _, settings, _, _ := newTestCmdService(t)
	s := &notifdom.Settings{ProfileID: "p1", MovieLevel: notifdom.LevelAll, SeriesLevel: notifdom.LevelAll, MusicLevel: notifdom.LevelAll}
	require.NoError(t, svc.UpdateSettings(context.Background(), s))
	got, err := settings.GetByProfile(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, notifdom.LevelAll, got.MovieLevel)
}

func TestCommandService_UpdateSettings_Invalid(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestCmdService(t)
	err := svc.UpdateSettings(context.Background(), &notifdom.Settings{ProfileID: "p1", MovieLevel: "bogus", SeriesLevel: notifdom.LevelAll, MusicLevel: notifdom.LevelAll})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid notification settings")
}

func TestCommandService_UpdateGlobalSettings_Valid(t *testing.T) {
	t.Parallel()
	svc, _, settings, _, _ := newTestCmdService(t)
	s := &notifdom.Settings{MovieLevel: notifdom.LevelAll, SeriesLevel: notifdom.LevelAll, MusicLevel: notifdom.LevelDisabled}
	require.NoError(t, svc.UpdateGlobalSettings(context.Background(), s))
	got, err := settings.GetGlobalDefaults(context.Background())
	require.NoError(t, err)
	assert.Equal(t, notifdom.LevelDisabled, got.MusicLevel)
}

func TestCommandService_UpdateGlobalSettings_Invalid(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestCmdService(t)
	err := svc.UpdateGlobalSettings(context.Background(), &notifdom.Settings{MovieLevel: notifdom.LevelAll, SeriesLevel: "nope", MusicLevel: notifdom.LevelAll})
	require.Error(t, err)
}

func TestCommandService_CleanupExpired_RemovesExpired(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, _ := newTestCmdService(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	require.NoError(t, repo.Create(ctx, &notifdom.Notification{ID: "old", ProfileID: "p1", ExpiresAt: &past, CreatedAt: time.Now()}))
	require.NoError(t, repo.Create(ctx, &notifdom.Notification{ID: "new", ProfileID: "p1", ExpiresAt: &future, CreatedAt: time.Now()}))
	require.NoError(t, svc.CleanupExpired(ctx))
	items, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "new", items[0].ID)
}

func TestCommandService_StartCleanup_ExitsOnCtxDone(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestCmdService(t)
	ctx, cancel := context.WithCancel(context.Background())
	svc.StartCleanup(ctx)
	cancel()
	// Wait deterministically for the cancellation to propagate, then yield
	// the scheduler so the cleanup goroutine has a chance to observe
	// ctx.Done() and return. -race covers actual leak detection.
	<-ctx.Done()
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
}

func TestCommandService_Start_StopsOnCtxDone(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestCmdService(t)
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)
	cancel()
	// Wait deterministically for the cancellation to propagate, then yield
	// the scheduler so the dedupe-cleanup goroutine can observe ctx.Done()
	// and return. -race covers actual leak detection.
	<-ctx.Done()
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
}

func TestCommandService_Start_StopsOnStop(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestCmdService(t)
	svc.Start(context.Background())
	svc.Stop()
	// Stop is idempotent
	svc.Stop()
}

func TestCommandService_UpsertPushSubscription_NoSubsRepo_NoOp(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestCmdService(t)
	err := svc.UpsertPushSubscription(context.Background(), "u1", "p1", "https://e", "k", "a")
	assert.NoError(t, err)
}

func TestCommandService_PushSubscription_WithRepo_RoundTrip(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	push := notiffakes.NewFakePushSender()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus, appnotification.WithPush(push, subs))
	ctx := context.Background()

	require.NoError(t, svc.UpsertPushSubscription(ctx, "u1", "p1", "https://e1", "k", "a"))
	require.NoError(t, svc.UpsertPushSubscription(ctx, "u2", "p2", "https://e2", "k", "a"))
	got, err := subs.GetByProfile(ctx, "p1")
	require.NoError(t, err)
	assert.Len(t, got, 1)

	require.NoError(t, svc.DeletePushByEndpoint(ctx, "u1", "https://e1"))
	got, err = subs.GetByProfile(ctx, "p1")
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, svc.DeletePushByUser(ctx, "u2"))
	got, err = subs.ListByUser(ctx, "u2")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCommandService_MediaItemAdded_PublishesNotificationCreatedEvent(t *testing.T) {
	t.Parallel()
	svc, _, _, profileRepo, bus := newTestCmdService(t)
	ctx := context.Background()
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"}))
	svc.Register()

	require.NoError(t, bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "Some Movie", Type: "movie", OccurredAt: time.Now(),
	}))

	var found bool
	for _, e := range bus.PublishedEvents() {
		if nc, ok := e.(notifdom.NotificationCreated); ok {
			found = true
			assert.Equal(t, notifdom.TypeNewMovie, nc.Type)
			assert.Equal(t, shared.ProfileID("p1"), nc.ProfileID)
		}
	}
	assert.True(t, found, "expected NotificationCreated event on bus")
}

func TestCommandService_MediaItemAdded_SkipsImageTypes(t *testing.T) {
	t.Parallel()
	svc, repo, _, profileRepo, bus := newTestCmdService(t)
	ctx := context.Background()
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"}))
	svc.Register()

	require.NoError(t, bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "img", Type: "photo", OccurredAt: time.Now(),
	}))
	require.NoError(t, bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m2", LibraryID: "lib1", Title: "img2", Type: "image", OccurredAt: time.Now(),
	}))

	items, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestCommandService_MediaItemAdded_AnalyticsErrorStillNotifiesLevelAll(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	ctx := context.Background()

	// p_rel uses genre-relevance (forces the favorite-genres analytics batch call);
	// p_all is LevelAll (genre-independent).
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p_rel", UserID: "u1", Name: "Rel"}))
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p_all", UserID: "u2", Name: "All"}))
	require.NoError(t, settings.Upsert(ctx, &notifdom.Settings{ProfileID: "p_rel", MovieLevel: notifdom.LevelRelevant, SeriesLevel: notifdom.LevelAll, MusicLevel: notifdom.LevelAll}))
	require.NoError(t, settings.Upsert(ctx, &notifdom.Settings{ProfileID: "p_all", MovieLevel: notifdom.LevelAll, SeriesLevel: notifdom.LevelAll, MusicLevel: notifdom.LevelAll}))

	// Genre-favorites analytics is down: this must NOT suppress notifications for
	// every profile - LevelAll profiles don't use genre relevance.
	analyzer.Err = errors.New("analytics down")
	svc.Register()

	require.NoError(t, bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "Some Movie", Type: "movie",
		Genres: []string{"Action"}, OccurredAt: time.Now(),
	}))

	var allNotified, relNotified bool
	for _, e := range bus.PublishedEvents() {
		if nc, ok := e.(notifdom.NotificationCreated); ok {
			switch nc.ProfileID {
			case shared.ProfileID("p_all"):
				allNotified = true
			case shared.ProfileID("p_rel"):
				relNotified = true
			}
		}
	}
	assert.True(t, allNotified, "LevelAll profile must still be notified when genre analytics fails")
	assert.False(t, relNotified, "Relevant profile fails-closed to no genre match when analytics is down")
}

func TestCommandService_FlushBatches_FlushesPending(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus, appnotification.WithBatchDelay(10*time.Second))
	ctx := context.Background()
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"}))
	svc.Start(ctx)
	defer svc.Stop()
	svc.Register()

	// series event is batched (waits 10s) - FlushBatches forces immediate emit
	require.NoError(t, bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "s1", LibraryID: "lib1", Title: "Some Series", Type: "series", OccurredAt: time.Now(),
	}))

	items, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, items, "batched event should not be emitted before flush")

	svc.FlushBatches()
	// Wait deterministically for async persistAndDispatch to materialize
	// the batched notification. Replaces a hand-rolled poll-with-sleep loop.
	require.Eventually(t, func() bool {
		got, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
		if err != nil {
			return false
		}
		items = got
		return len(items) > 0
	}, 2*time.Second, 5*time.Millisecond)
	assert.Len(t, items, 1)
}

// A title leaving the library takes its notifications with it, for every
// profile: they used to outlive it (no foreign key on media_item_id) and open
// a media page that answers 404.
func TestCommandService_MediaItemRemoved_DeletesItsNotifications(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, bus := newTestCmdService(t)
	ctx := context.Background()
	for _, n := range []*notifdom.Notification{
		{ID: "n-gone-1", ProfileID: "p1", Type: notifdom.TypeNewMovie, Title: "Gone", MediaItemID: "m-gone", Level: notifdom.LevelAll, CreatedAt: time.Now()},
		{ID: "n-gone-2", ProfileID: "p2", Type: notifdom.TypeNewMovie, Title: "Gone", MediaItemID: "m-gone", Level: notifdom.LevelAll, CreatedAt: time.Now()},
		{ID: "n-keep", ProfileID: "p1", Type: notifdom.TypeNewMovie, Title: "Keep", MediaItemID: "m-keep", Level: notifdom.LevelAll, CreatedAt: time.Now()},
	} {
		require.NoError(t, repo.Create(ctx, n))
	}
	svc.Register()

	require.NoError(t, bus.Publish(ctx, libevent.MediaItemRemoved{MediaItemID: "m-gone", LibraryID: "lib1", OccurredAt: time.Now()}))

	p1, err := repo.ListByProfile(ctx, "p1", false, 10, 0)
	require.NoError(t, err)
	require.Len(t, p1, 1)
	assert.Equal(t, "n-keep", p1[0].ID)
	p2, err := repo.ListByProfile(ctx, "p2", false, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, p2)
}
