package notification_test

// This file guards against the episode-notification dedup N+1: emitNotifications
// used to call FindRecentByProfileTypeAndSeries twice PER PROFILE (once for
// parent series/season notifs, once for grouped episode notifs) inside the
// profiles loop. On a library scan with many series x many profiles that was
// ~2 x series x profiles round-trips. The fix preloads once per series flush
// via FindRecentByProfilesTypeAndSeries and decides in memory.

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

// countingNotifRepo wraps FakeNotificationRepository and counts calls to the
// batched dedup preload, so tests can assert the query count directly instead
// of inferring it from side effects.
type countingNotifRepo struct {
	*notiffakes.FakeNotificationRepository
	mu         sync.Mutex
	batchCalls int
	failNext   bool
}

func (c *countingNotifRepo) FindRecentByProfilesTypeAndSeries(ctx context.Context, profileIDs []shared.ProfileID, types []notifdom.Type, seriesKey notifdom.SeriesKey, since time.Time) (map[shared.ProfileID][]*notifdom.Notification, error) {
	c.mu.Lock()
	c.batchCalls++
	fail := c.failNext
	c.mu.Unlock()
	if fail {
		return nil, errors.New("preload boom")
	}
	return c.FakeNotificationRepository.FindRecentByProfilesTypeAndSeries(ctx, profileIDs, types, seriesKey, since)
}

func (c *countingNotifRepo) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.batchCalls
}

// TestEpisodeNotifDedup_OneBatchQueryPerSeriesFlush proves the N+1 is gone:
// with M profiles, a single series flush must trigger exactly ONE call to the
// batched preload, not one call per profile (and not two, since parent-type
// and episode-type dedup are now folded into the same preloaded query).
func TestEpisodeNotifDedup_OneBatchQueryPerSeriesFlush(t *testing.T) {
	t.Parallel()
	repo := &countingNotifRepo{FakeNotificationRepository: notiffakes.NewFakeNotificationRepository()}
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	const profileCount = 10
	for i := range profileCount {
		id := fmt.Sprintf("p%d", i)
		require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: shared.ProfileID(id), UserID: shared.UserID("u" + id), Name: id}))
	}

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	// One episode notification fan-out (single series flush).
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1", LibraryID: "lib1",
		Title: "S01E01", SeriesTitle: "Foo", Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	assert.Equal(t, 1, repo.calls(), "one series flush across N profiles must issue exactly one preload query, not one per profile")

	// A second, distinct series flush must add exactly one more call (N series -> N calls).
	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep2", LibraryID: "lib1",
		Title: "S01E01", SeriesTitle: "Bar", Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	assert.Equal(t, 2, repo.calls(), "second series flush must add exactly one more preload call")

	// Sanity: every profile actually got its notification (the fix didn't break fan-out).
	for i := range profileCount {
		notifs, err := repo.ListByProfile(ctx, shared.ProfileID(fmt.Sprintf("p%d", i)), false, 0, 0)
		require.NoError(t, err)
		assert.Len(t, notifs, 2, "profile %d should have one notification per series", i)
	}
}

// TestEpisodeNotifDedup_PreloadFailureFailsOpen proves the preload keeps the
// pre-existing fail-open contract: if the batched dedup query itself errors,
// every profile in that fan-out still gets its notification created (a
// duplicate notification is preferred over a silently dropped one).
func TestEpisodeNotifDedup_PreloadFailureFailsOpen(t *testing.T) {
	t.Parallel()
	repo := &countingNotifRepo{FakeNotificationRepository: notiffakes.NewFakeNotificationRepository(), failNext: true}
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"}))
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p2", UserID: "u2", Name: "Bob"}))

	// Seed an existing series notification that WOULD normally cause a skip
	// (case A/B) - if the preload error were instead treated as "skip", these
	// profiles would wrongly get nothing.
	expires := time.Now().AddDate(0, 0, 30)
	require.NoError(t, repo.Create(ctx, &notifdom.Notification{
		ID: "series-notif", ProfileID: "p1", Type: notifdom.TypeNewSeries,
		Title: "New Series: Foo", SeriesKey: "lib1:Foo",
		Level: notifdom.LevelAll, CreatedAt: time.Now(), ExpiresAt: &expires,
	}))

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "ep1", LibraryID: "lib1",
		Title: "S01E01", SeriesTitle: "Foo", Type: "episode", OccurredAt: time.Now(),
	})
	svc.FlushBatches()

	require.Equal(t, 1, repo.calls(), "preload must have been attempted exactly once")

	for _, p := range []shared.ProfileID{"p1", "p2"} {
		notifs, err := repo.ListByProfile(ctx, p, false, 0, 0)
		require.NoError(t, err)
		episodeCount := 0
		for _, n := range notifs {
			if n.Type == notifdom.TypeNewEpisode {
				episodeCount++
			}
		}
		assert.Equal(t, 1, episodeCount, "preload failure must fail open: profile %s must still get its episode notification created", p)
	}
}

// TestBatchEviction_FlushesInsteadOfDiscarding pins the capacity contract: when
// the batch map hits maxBatchEntries, the evicted (oldest) batch must FIRE its
// pending notification early - before the fix, eviction deleted the entry and
// the user never heard about that series at all.
func TestBatchEviction_FlushesInsteadOfDiscarding(t *testing.T) {
	t.Parallel()
	repo := &countingNotifRepo{FakeNotificationRepository: notiffakes.NewFakeNotificationRepository()}
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "p1"}))

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus,
		appnotification.WithBatchDelay(time.Hour)) // timers must never fire on their own here
	svc.Register()

	// Fill the map to capacity, then one more: the OLDEST batch gets evicted.
	for i := range 1001 {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: shared.MediaItemID(fmt.Sprintf("ep%d", i)), LibraryID: "lib1",
			Title: fmt.Sprintf("S01E0%d", i%9+1), SeriesTitle: fmt.Sprintf("Series-%d", i),
			Type: "episode", OccurredAt: time.Now(),
		})
	}

	// The evicted batch flushes on a detached goroutine - wait for its
	// notification instead of a sleep.
	require.Eventually(t, func() bool {
		notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
		if err != nil {
			return false
		}
		for _, n := range notifs {
			if n.Subject == "Series-0" {
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond,
		"the evicted oldest batch (Series-0) must emit its notification, not vanish")
}

// The SÉRIE flush path must ship the grouping fields as DATA on the stored
// notification (Subject + SeasonCount + EpisodeCount), so a client words the
// sentence itself - the track path is covered elsewhere, this pins the series
// sibling.
func TestSeriesFlush_StoresSubjectAndCounts(t *testing.T) {
	t.Parallel()
	repo := &countingNotifRepo{FakeNotificationRepository: notiffakes.NewFakeNotificationRepository()}
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "p1"}))

	svc := appnotification.NewCommandService(repo, settings, analyzer, profileRepo, bus,
		appnotification.WithBatchDelay(time.Hour))
	svc.Register()

	publish := func(id, title string, typ libevent.MediaItemType) {
		bus.Publish(ctx, libevent.MediaItemAdded{
			MediaItemID: shared.MediaItemID(id), LibraryID: "lib1",
			Title: title, SeriesTitle: "Some Series", Type: typ, OccurredAt: time.Now(),
		})
	}
	publish("ser1", "Some Series", "series")
	publish("sea1", "Season 1", "season")
	publish("sea2", "Season 2", "season")
	publish("ep1", "S01E01", "episode")
	publish("ep2", "S01E02", "episode")
	publish("ep3", "S02E01", "episode")
	svc.FlushBatches()

	notifs, err := repo.ListByProfile(ctx, "p1", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, notifs, 1, "one grouped notification for the whole series batch")
	got := notifs[0]
	assert.Equal(t, "Some Series", got.Subject)
	assert.Equal(t, 2, got.SeasonCount)
	assert.Equal(t, 3, got.EpisodeCount)
	assert.Equal(t, notifdom.NewSeriesKey("lib1", "Some Series"), got.SeriesKey)
}
