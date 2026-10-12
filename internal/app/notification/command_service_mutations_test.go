package notification_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	appnotification "github.com/eveyproject/evey/internal/app/notification"
	"github.com/eveyproject/evey/internal/domain/backup"
	libevent "github.com/eveyproject/evey/internal/domain/library"
	notifdom "github.com/eveyproject/evey/internal/domain/notification"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var slogMu sync.Mutex

// syncBuffer guards a bytes.Buffer: detached service goroutines (evicted
// batch flushes) keep logging while a test reads, and a bare buffer races
// under -race.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureSlogBuffer redirects slog output to a buffer; returns buffer + restore fn.
func captureSlogBuffer(t *testing.T) (*syncBuffer, func()) {
	t.Helper()
	slogMu.Lock()
	var buf syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &buf, func() {
		slog.SetDefault(prev)
		slogMu.Unlock()
	}
}

// --- wrappers for targeted error injection ---

type pubErrBus struct {
	*sharedfakes.FakeEventBus
	pubErr error
}

func (b *pubErrBus) Publish(ctx context.Context, event shared.Event) error {
	// If it's the new notification.created event, fail. Otherwise delegate.
	if _, ok := event.(notifdom.NotificationCreated); ok {
		return b.pubErr
	}
	return b.FakeEventBus.Publish(ctx, event)
}

type cleanupErrRepo struct {
	*notiffakes.FakeNotificationRepository
	cleanupErr error
	called     chan struct{}
	once       sync.Once
}

func (r *cleanupErrRepo) CleanupExpired(ctx context.Context) error {
	r.once.Do(func() { close(r.called) })
	return r.cleanupErr
}

// TestPublishErr_Logged covers command_service.go:446 (pubErr != nil) branch.
func TestNotificationService_PublishNotificationCreatedError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	inner := sharedfakes.NewFakeEventBus()
	bus := &pubErrBus{FakeEventBus: inner, pubErr: errors.New("publish boom")}

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	inner.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "Some Movie", Type: "movie", OccurredAt: time.Now(),
	})

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Contains(t, buf.String(), "publish notification.created")
}

// TestPushSendError_LogsWarning covers command_service.go:461 (sendErr != nil).
func TestNotificationService_PushSendError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	pushSender.Err = errors.New("push fail")
	subRepo := notiffakes.NewFakePushSubscriptionRepository()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/1", "k", "a")

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	appnotification.WithPush(pushSender, subRepo)(svc)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "Some Movie", Type: "movie", OccurredAt: time.Now(),
	})

	assert.Contains(t, buf.String(), "webpush: send failed")
}

// TestNotificationService_PushGone410_PrunesSubscription covers sendPush's 410
// cleanup: a permanently-gone endpoint is deleted, not retried on every push.
func TestNotificationService_PushGone410_PrunesSubscription(t *testing.T) {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	pushSender.Err = notifdom.ErrSubscriptionGone
	subRepo := notiffakes.NewFakePushSubscriptionRepository()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/1", "k", "a")

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	appnotification.WithPush(pushSender, subRepo)(svc)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "Some Movie", Type: "movie", OccurredAt: time.Now(),
	})

	got, err := subRepo.GetByProfile(ctx, "p1")
	require.NoError(t, err)
	assert.Empty(t, got, "a 410 Gone subscription should have been pruned")
}

// TestPushGetSubsError_LogsWarning covers get subscriptions error path.
func TestNotificationService_PushGetSubsError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	pushSender := notiffakes.NewFakePushSender()
	subRepo := notiffakes.NewFakePushSubscriptionRepository()
	subRepo.Err = errors.New("db down")

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	appnotification.WithPush(pushSender, subRepo)(svc)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "Some Movie", Type: "movie", OccurredAt: time.Now(),
	})

	assert.Contains(t, buf.String(), "webpush: get subscriptions failed")
}

// TestCleanupExpiredError_LogsWarning covers command_service.go:535 (err != nil).
func TestNotificationService_CleanupExpiredError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	inner := notiffakes.NewFakeNotificationRepository()
	repo := &cleanupErrRepo{
		FakeNotificationRepository: inner,
		cleanupErr:                 errors.New("cleanup boom"),
		called:                     make(chan struct{}),
	}
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	ctx, cancel := context.WithCancel(context.Background())
	svc.StartCleanup(ctx)

	select {
	case <-repo.called:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("CleanupExpired was not called")
	}
	// Give slog write a moment, then cancel to stop the goroutine.
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	assert.Contains(t, buf.String(), "notification cleanup")
}

// TestCleanupExpiredDirect covers CleanupExpired method.
func TestNotificationService_CleanupExpiredDirect(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	require.NoError(t, svc.CleanupExpired(context.Background()))

	repo.Err = errors.New("bang")
	require.Error(t, svc.CleanupExpired(context.Background()))
}

// TestTrackBatchedByAlbum covers command_service.go:177/199/201 (track with SeriesTitle).
func TestNotificationService_TrackBatchedByAlbum(t *testing.T) {
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

	// Tracks with SeriesTitle batch under album:lib:SeriesTitle.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "t1", LibraryID: "lib1", Title: "Track 1", SeriesTitle: "Album A", Type: "track", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "t2", LibraryID: "lib1", Title: "Track 2", SeriesTitle: "Album A", Type: "track", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "tracks with SeriesTitle should batch into one notification")
	// Tracks without an album event must still be an ALBUM notification, not an
	// episode one: before the routing fix these fell through the episode branch
	// (wrong type, wrong "New Episode" label, music governed by the episode
	// level). Assert the type and the album label, not just the count.
	assert.Equal(t, notifdom.TypeNewAlbum, notifs[0].Type)
	assert.Equal(t, "Album A", notifs[0].Subject)
	assert.Equal(t, 2, notifs[0].EpisodeCount)
	assert.Contains(t, notifs[0].Title, "2 tracks")
}

// A single track added to an existing album (no album event in the batch) must
// still surface an album notification - it used to be dropped silently because
// the episode branch emitted type "track", which typeAndTitle maps to "".
func TestNotificationService_SingleTrackWithoutAlbumEvent_NotDropped(t *testing.T) {
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
		MediaItemID: "t1", LibraryID: "lib1", Title: "Track 1", SeriesTitle: "Album A", Type: "track", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "a lone track must not be dropped")
	assert.Equal(t, notifdom.TypeNewAlbum, notifs[0].Type)
	assert.Equal(t, "Album A", notifs[0].Subject)
	assert.Contains(t, notifs[0].Title, "1 track")
	assert.NotContains(t, notifs[0].Title, "1 tracks")
}

// An untagged track (no SeriesTitle - the real prod case of an untagged audio
// documentary) used to notify NOTHING: it fell past the buffer (which required
// SeriesTitle != "") to the direct emit, where typeAndTitle maps type "track"
// to "" and drops it. It must still surface an album notification named by the
// track's own title.
func TestNotificationService_UntaggedTrack_StillNotifies(t *testing.T) {
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
		MediaItemID: "t1", LibraryID: "lib1", Title: "Field Recording", SeriesTitle: "", Type: "track", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "an untagged track must still produce a notification")
	assert.Equal(t, notifdom.TypeNewAlbum, notifs[0].Type)
	assert.Equal(t, "Field Recording", notifs[0].Subject)
}

// TestTrackWithoutSeriesTitle_UsesTitleAsKey covers service.go batchKey fallback
// and service.go:69 (trackCount > 0 boundary) where trackCount==0 produces plain title.
func TestNotificationService_AlbumNoTracks_PlainTitle(t *testing.T) {
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
		MediaItemID: "a1", LibraryID: "lib1", Title: "Lonely Album", Type: "album", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	// No trackCount => plain "new_album:Lonely Album" (no "N tracks" suffix).
	assert.Equal(t, "new_album:Lonely Album", notifs[0].Title)
	assert.NotContains(t, notifs[0].Title, "tracks")
}

// TestAlbumWithTracks_IncludesTrackCount covers service.go:69 trackCount>0 branch.
func TestNotificationService_AlbumWithTracks_IncludesTrackCount(t *testing.T) {
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
		MediaItemID: "a1", LibraryID: "lib1", Title: "The Album", Type: "album", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "t1", LibraryID: "lib1", Title: "T1", SeriesTitle: "The Album", Type: "track", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "t2", LibraryID: "lib1", Title: "T2", SeriesTitle: "The Album", Type: "track", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Contains(t, notifs[0].Title, "2 tracks")
}

// TestSettingsGetError_LogsWarning exercises the get settings batch error branch.
func TestNotificationService_GetSettingsBatchError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	settings.Err = errors.New("db down")
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "X", Type: "movie", OccurredAt: time.Now(),
	})

	assert.Contains(t, buf.String(), "get settings batch failed")
}

// TestProfileListError_LogsWarning covers profile list error branch.
func TestNotificationService_ProfileListError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	bus := sharedfakes.NewFakeEventBus()
	profiles := &errProfileLister{err: errors.New("profiles down")}

	svc := appnotification.NewCommandService(repo, settings, analyzer, profiles, bus)
	svc.Register()

	bus.Publish(context.Background(), libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "X", Type: "movie", OccurredAt: time.Now(),
	})

	assert.Contains(t, buf.String(), "list profiles failed")
}

// TestFavoriteGenresBatchError_LogsWarning covers analyzer error branch.
func TestNotificationService_FavoriteGenresBatchError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Err = errors.New("analyzer down")
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:  "p1",
		MovieLevel: notifdom.LevelRelevant,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "X", Type: "movie", Genres: []string{"Action"}, OccurredAt: time.Now(),
	})

	assert.Contains(t, buf.String(), "batch get favorite genres failed")
}

// TestRepoCreateError_LogsWarning covers persistAndDispatch error branch.
func TestNotificationService_RepoCreateError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	repo.Err = errors.New("create boom")
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "X", Type: "movie", OccurredAt: time.Now(),
	})

	assert.Contains(t, buf.String(), "create notification failed")
}

// TestBackupFailed_ProfilesError_LogsWarning covers handleBackupFailed err path.
func TestNotificationService_BackupFailed_ProfilesError_LogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profiles := &errProfileLister{err: errors.New("profiles down")}
	bus := sharedfakes.NewFakeEventBus()

	svc := appnotification.NewCommandService(repo, settings, analyzer, profiles, bus)
	svc.Register()

	bus.Publish(context.Background(), backup.BackupFailed{Error: "disk full", OccurredAt: time.Now()})

	assert.Contains(t, buf.String(), "list profiles for backup.failed")
}

// TestRelevantLevel_EmptyGenres_SkipsFavoriteFetch covers command_service.go:395
// (len(genres) > 0 guard inside fetchFavoriteGenres).
func TestNotificationService_RelevantLevel_EmptyGenres_NoAnalyzerCall(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	// Analyzer returns err to prove it's NOT called when genres==nil.
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Err = errors.New("should-not-be-called")
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settings.Upsert(ctx, &notifdom.Settings{
		ProfileID:  "p1",
		MovieLevel: notifdom.LevelRelevant,
	})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	// No genres => analyzer must not be invoked (because len(genres)==0 bypasses relevance fetch).
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "X", Type: "movie", OccurredAt: time.Now(),
	})

	// shouldNotify returns false when genres empty and level is relevance-based.
	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, notifs)
}

// TestFlushBatches_Idempotent covers FlushBatches with a batched entry.
func TestNotificationService_FlushBatches_StopsTimer(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	appnotification.WithBatchDelay(10 * time.Second)(svc)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "s1", LibraryID: "lib1", Title: "Some Series", Type: "series", OccurredAt: time.Now(),
	})

	// Call FlushBatches and confirm exactly one notification produced promptly.
	svc.FlushBatches()
	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)

	// Second call is a no-op (map empty), must not panic or duplicate.
	svc.FlushBatches()
	notifs, err = repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
}

// TestBatchEviction_AtLimit covers command_service.go:219 maxBatchEntries >= boundary.
// Uses a smaller drive (sentinel) to at least hit the loop; full 1000-entry test
// would be slow but we assert eviction occurs by adding >=1000 distinct keys.
func TestNotificationService_BatchEviction_AtLimit(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	appnotification.WithBatchDelay(1 * time.Hour)(svc)
	svc.Register()

	// Publish 1001 unique series to trigger eviction at 1000.
	for i := 0; i < 1001; i++ {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: shared.MediaItemID("sid-" + itoa(i)),
			LibraryID:   "lib1",
			Title:       "Series-" + itoa(i),
			Type:        "series",
			OccurredAt:  time.Now(),
		})
	}

	// Assert at least one eviction warning logged.
	assert.True(t, strings.Contains(buf.String(), "notification batch map full"),
		"expected eviction warning in slog output, got: %s", buf.String())

	// Flush remaining to avoid leaking timers.
	svc.FlushBatches()
	svc.Stop()
}

func itoa(i int) string {
	// tiny local helper to avoid strconv import churn
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// countingAnalyzer wraps FakeWatchHistoryAnalyzer and counts GetFavoriteGenresBatch calls.
type countingAnalyzer struct {
	*notiffakes.FakeWatchHistoryAnalyzer
	mu         sync.Mutex
	batchCalls int
}

func (c *countingAnalyzer) GetFavoriteGenresBatch(ctx context.Context, profileIDs []shared.ProfileID, minCount int) (map[shared.ProfileID][]string, error) {
	c.mu.Lock()
	c.batchCalls++
	c.mu.Unlock()
	return c.FakeWatchHistoryAnalyzer.GetFavoriteGenresBatch(ctx, profileIDs, minCount)
}

func (c *countingAnalyzer) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.batchCalls
}

// TestNotif_KillMutant_BatchedEpisodesUsePluralPushTitle kills the mutant at
// command_service.go:329 (pushTitleOverride != ""). If the guard is negated,
// the override is ignored and the push title becomes the singular default
// "New Episode: ..." instead of the batched plural "New Episodes: ...".
func TestNotif_KillMutant_BatchedEpisodesUsePluralPushTitle(t *testing.T) {
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
	_ = subRepo.Upsert(ctx, "u1", "p1", "https://push.example.com/1", "k", "a")

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	appnotification.WithPush(pushSender, subRepo)(svc)
	svc.Register()

	// Two episodes of the same series batch into one notification with plural push title.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "e1", LibraryID: "lib1", Title: "Ep 1", SeriesTitle: "Some Series", Type: "episode", OccurredAt: time.Now(),
	})
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "e2", LibraryID: "lib1", Title: "Ep 2", SeriesTitle: "Some Series", Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	require.Len(t, pushSender.Calls, 1)
	// Must be the OVERRIDE plural title; the non-override default would be "New Episode: ...".
	assert.True(t, strings.HasPrefix(pushSender.Calls[0].Title, "New Episodes: "),
		"expected plural override push title, got %q", pushSender.Calls[0].Title)
}

// TestNotif_KillMutant_EmptyGenresSkipsAnalyzerBatch kills the mutant at
// command_service.go:395 (len(genres) > 0 boundary). If the condition flips to
// >= 0, the analyzer batch call is made even when genres are empty.
func TestNotif_KillMutant_EmptyGenresSkipsAnalyzerBatch(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settingsRepo := notiffakes.NewFakeSettingsRepository()
	wrapped := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer := &countingAnalyzer{FakeWatchHistoryAnalyzer: wrapped}
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = settingsRepo.Upsert(ctx, &notifdom.Settings{
		ProfileID:  "p1",
		MovieLevel: notifdom.LevelRelevant,
	})

	svc := appnotification.NewCommandService(repo, settingsRepo, analyzer, profileRepo, bus)
	svc.Register()

	// Genres is nil/empty: the analyzer batch call must NOT happen.
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "m1", LibraryID: "lib1", Title: "X", Type: "movie", OccurredAt: time.Now(),
	})

	assert.Equal(t, 0, analyzer.calls(), "analyzer batch must not be called when genres empty")
}

// TestNotif_KillMutant_StartCleanupNoPanic_NoRecoverLog kills the mutant at
// command_service.go:531 (r != nil in recover). If the guard is negated, the
// recovered-panic slog line is emitted on every normal goroutine exit too.
func TestNotif_KillMutant_StartCleanupNoPanic_NoRecoverLog(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)

	ctx, cancel := context.WithCancel(context.Background())
	svc.StartCleanup(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	assert.NotContains(t, buf.String(), "recovered panic in notification cleanup",
		"recover branch must not log when no panic occurred")
}
