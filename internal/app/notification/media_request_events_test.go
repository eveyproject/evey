package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appnotification "github.com/evey-project/evey/internal/app/notification"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	notifdom "github.com/evey-project/evey/internal/domain/notification"
	notiffakes "github.com/evey-project/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
)

// A new media request pages the operators only; the requester hears back
// on the answer, with the decision and the operator's note.
func TestNotificationService_MediaRequestEvents(t *testing.T) {
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
		RequestID: "req-1", ProfileID: "p-bob", UserID: "u-bob", MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie", OccurredAt: time.Now(),
	})
	adminNotifs, err := repo.ListByProfile(ctx, "p-admin", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, adminNotifs, 1)
	assert.Equal(t, notifdom.TypeMediaRequest, adminNotifs[0].Type)
	assert.Contains(t, adminNotifs[0].Title, "Some Movie")
	assert.Equal(t, string(mediarequest.MediaTypeMovie), adminNotifs[0].Message)
	assert.NotNil(t, adminNotifs[0].ExpiresAt, "operator pages expire, they are not kept forever")
	bobNotifs, err := repo.ListByProfile(ctx, "p-bob", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, bobNotifs, "the requester is not paged about their own request")

	bus.Publish(ctx, mediarequest.RequestAnswered{
		RequestID: "req-1", ProfileID: "p-bob", Title: "Some Movie", Status: mediarequest.StatusApproved, Note: "added to the queue", OccurredAt: time.Now(),
	})
	bobNotifs, err = repo.ListByProfile(ctx, "p-bob", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, bobNotifs, 1)
	assert.Equal(t, notifdom.TypeRequestAnswered, bobNotifs[0].Type)
	assert.Contains(t, bobNotifs[0].Title, "Some Movie")
	assert.Contains(t, bobNotifs[0].Message, string(mediarequest.StatusApproved))
	assert.Contains(t, bobNotifs[0].Message, "added to the queue")
	adminNotifs, _ = repo.ListByProfile(ctx, "p-admin", false, 0, 0)
	assert.Len(t, adminNotifs, 1, "the answer goes to the requester only")

	// A requester whose profile vanished in between: nothing to deliver, no panic.
	bus.Publish(ctx, mediarequest.RequestAnswered{RequestID: "req-2", ProfileID: "p-gone", Title: "X", Status: mediarequest.StatusRejected})
	gone, err := repo.ListByProfile(ctx, "p-gone", false, 0, 0)
	require.NoError(t, err)
	assert.Len(t, gone, 1, "the row is still written for the profile id; only the push locale is unresolved")
}

// The answer's push is written for the REQUESTER: in their profile's language,
// not the operator's who answered nor the server default.
func TestNotificationService_RequestAnsweredPushSpeaksTheRequesterLanguage(t *testing.T) {
	t.Parallel()
	tr, err := eveyi18n.NewFromDir("../../../i18n")
	require.NoError(t, err)
	ctx := context.Background()
	profileRepo := userfakes.NewFakeProfileRepository()
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p-admin", UserID: "u-admin", Name: "Root", Locale: "en"})
	_ = profileRepo.Create(ctx, &user.Profile{ID: "p-bob", UserID: "u-bob", Name: "Bob", Locale: "fr"})
	subs := notiffakes.NewFakePushSubscriptionRepository()
	require.NoError(t, subs.Upsert(ctx, "u-bob", "p-bob", "https://push.invalid/bob", "k", "a"))
	sender := notiffakes.NewFakePushSender()
	bus := sharedfakes.NewFakeEventBus()

	svc := appnotification.NewCommandService(notiffakes.NewFakeNotificationRepository(), notiffakes.NewFakeSettingsRepository(),
		notiffakes.NewFakeWatchHistoryAnalyzer(), profileRepo, bus,
		appnotification.WithPush(sender, subs), appnotification.WithTranslator(tr))
	svc.Register()

	bus.Publish(ctx, mediarequest.RequestAnswered{
		RequestID: "req-1", ProfileID: "p-bob", Title: "Some Movie", Status: mediarequest.StatusApproved, OccurredAt: time.Now(),
	})
	require.Len(t, sender.Calls, 1)
	assert.Equal(t, "Votre demande de média a été mise à jour : Some Movie", sender.Calls[0].Title)
}
