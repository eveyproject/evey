package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appnotification "github.com/eveyproject/evey/internal/app/notification"
	"github.com/eveyproject/evey/internal/domain/mediarequest"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

// Seen on 2026-10-07 in a French UI: "New media request: Some Movie" over
// "movie", and "Status: approved - ..." for the answer. The stored row keeps
// its English title and machine message for older clients, but now carries the
// SUBJECT so a current client words the sentence itself; and the push - the
// one copy rendered server-side - is localised body included.
func TestNotificationService_MediaRequestRowsCarryTheSubject(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	ctx := context.Background()
	admin := &user.Profile{ID: "p-admin", UserID: "u-admin", Name: "Root"}
	requester := &user.Profile{ID: "p-bob", UserID: "u-bob", Name: "Bob", Locale: "fr"}
	_ = profileRepo.Create(ctx, admin)
	_ = profileRepo.Create(ctx, requester)

	svc := appnotification.NewCommandService(repo, notiffakes.NewFakeSettingsRepository(),
		notiffakes.NewFakeWatchHistoryAnalyzer(), profileRepo, bus,
		appnotification.WithAdminLister(&fakeAdminLister{profiles: []*user.Profile{admin}}))
	svc.Register()

	bus.Publish(ctx, mediarequest.RequestCreated{
		RequestID: "req-1", ProfileID: "p-bob", UserID: "u-bob", MediaType: mediarequest.MediaTypeTV, Title: "Some Series", OccurredAt: time.Now(),
	})
	adminNotifs, err := repo.ListByProfile(ctx, "p-admin", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, adminNotifs, 1)
	assert.Equal(t, "Some Series", adminNotifs[0].Subject)
	assert.Equal(t, "New media request: Some Series", adminNotifs[0].Title, "older clients still read the English label")
	assert.Equal(t, "tv", adminNotifs[0].Message, "the media type stays a machine value the client translates")

	bus.Publish(ctx, mediarequest.RequestAnswered{
		RequestID: "req-1", ProfileID: "p-bob", Title: "Some Series", Status: mediarequest.StatusRejected, Note: "déjà là", OccurredAt: time.Now(),
	})
	bobNotifs, err := repo.ListByProfile(ctx, "p-bob", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, bobNotifs, 1)
	assert.Equal(t, "Some Series", bobNotifs[0].Subject)
	assert.Equal(t, "Status: rejected - déjà là", bobNotifs[0].Message, "the shape the client parses: status word ours, note verbatim")
}

func TestNotificationService_MediaRequestPushBodyIsLocalised(t *testing.T) {
	t.Parallel()
	tr, err := eveyi18n.NewFromDir("../../../i18n")
	require.NoError(t, err)
	ctx := context.Background()
	profileRepo := userfakes.NewFakeProfileRepository()
	admin := &user.Profile{ID: "p-admin", UserID: "u-admin", Name: "Root", Locale: "fr"}
	_ = profileRepo.Create(ctx, admin)
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p-bob", UserID: "u-bob", Name: "Bob", Locale: "fr"})
	subs := notiffakes.NewFakePushSubscriptionRepository()
	require.NoError(t, subs.Upsert(ctx, "u-admin", "p-admin", "https://push.invalid/admin", "k", "a"))
	require.NoError(t, subs.Upsert(ctx, "u-bob", "p-bob", "https://push.invalid/bob", "k", "a"))
	sender := notiffakes.NewFakePushSender()
	bus := sharedfakes.NewFakeEventBus()

	svc := appnotification.NewCommandService(notiffakes.NewFakeNotificationRepository(), notiffakes.NewFakeSettingsRepository(),
		notiffakes.NewFakeWatchHistoryAnalyzer(), profileRepo, bus,
		appnotification.WithAdminLister(&fakeAdminLister{profiles: []*user.Profile{admin}}),
		appnotification.WithPush(sender, subs), appnotification.WithTranslator(tr))
	svc.Register()

	bus.Publish(ctx, mediarequest.RequestCreated{
		RequestID: "req-1", ProfileID: "p-bob", UserID: "u-bob", MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie", OccurredAt: time.Now(),
	})
	require.Len(t, sender.Calls, 1)
	assert.Equal(t, "Nouvelle demande de média : Some Movie", sender.Calls[0].Title)
	assert.Equal(t, "Film", sender.Calls[0].Body, "the body is the media type, in the operator's language")

	bus.Publish(ctx, mediarequest.RequestAnswered{
		RequestID: "req-1", ProfileID: "p-bob", Title: "Some Movie", Status: mediarequest.StatusApproved, Note: "ce week-end", OccurredAt: time.Now(),
	})
	require.Len(t, sender.Calls, 2)
	assert.Equal(t, "Acceptée - ce week-end", sender.Calls[1].Body)

	bus.Publish(ctx, mediarequest.RequestAnswered{
		RequestID: "req-1", ProfileID: "p-bob", Title: "Some Movie", Status: mediarequest.StatusLater, OccurredAt: time.Now(),
	})
	require.Len(t, sender.Calls, 3)
	assert.Equal(t, "Plus tard", sender.Calls[2].Body, "no note, no dangling dash")
}
