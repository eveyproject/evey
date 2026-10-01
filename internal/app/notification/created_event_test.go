package notification_test

import (
	"context"
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

func createdEvents(bus *sharedfakes.FakeEventBus) []notifdom.NotificationCreated {
	var out []notifdom.NotificationCreated
	for _, ev := range bus.PublishedEvents() {
		if c, ok := ev.(notifdom.NotificationCreated); ok {
			out = append(out, c)
		}
	}
	return out
}

// Every per-profile row of one new title carries the same fan-out id and the
// push title as key + params, which is what the e-mail renders per account.
func TestNotificationCreated_OneFanoutPerTitle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	profileRepo := userfakes.NewFakeProfileRepository()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p2", UserID: "u1", Name: "Kids"})
	bus := sharedfakes.NewFakeEventBus()
	svc := appnotification.NewCommandService(notiffakes.NewFakeNotificationRepository(), notiffakes.NewFakeSettingsRepository(),
		notiffakes.NewFakeWatchHistoryAnalyzer(), profileRepo, bus)
	svc.Register()

	bus.Publish(ctx, libevent.MediaItemAdded{MediaItemID: "m1", LibraryID: "lib1", Title: "Big Buck Bunny", Type: "movie", OccurredAt: time.Now()})
	bus.Publish(ctx, libevent.MediaItemAdded{MediaItemID: "m2", LibraryID: "lib1", Title: "Sintel", Type: "movie", OccurredAt: time.Now()})

	evs := createdEvents(bus)
	require.Len(t, evs, 4)
	assert.NotEmpty(t, evs[0].FanoutID)
	assert.Equal(t, evs[0].FanoutID, evs[1].FanoutID, "two profiles of one title share the fan-out")
	assert.NotEqual(t, evs[0].FanoutID, evs[2].FanoutID, "another title is another fan-out")
	assert.Equal(t, "notifications.push.new_movie.title", evs[0].TitleKey)
	assert.Equal(t, map[string]string{"label": "Big Buck Bunny"}, evs[0].TitleParams)
	assert.Equal(t, "New Movie: Big Buck Bunny", evs[0].TitleFallback)
}

// A season landing episode by episode updates the grouped row in place: one
// notification.created (one e-mail), not one per episode.
func TestNotificationCreated_GroupedEpisodesPublishOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	profileRepo := userfakes.NewFakeProfileRepository()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice"})
	bus := sharedfakes.NewFakeEventBus()
	svc := appnotification.NewCommandService(notiffakes.NewFakeNotificationRepository(), notiffakes.NewFakeSettingsRepository(),
		notiffakes.NewFakeWatchHistoryAnalyzer(), profileRepo, bus)
	svc.Register()

	for _, ep := range []string{"S01E01", "S01E02", "S01E03"} {
		bus.Publish(ctx, libevent.MediaItemAdded{MediaItemID: shared.MediaItemID("id-" + ep), LibraryID: "lib1", Title: ep, SeriesTitle: "Some Series", Type: "episode", OccurredAt: time.Now()})
		svc.FlushBatches()
	}
	assert.Len(t, createdEvents(bus), 1)
}
