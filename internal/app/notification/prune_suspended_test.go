package notification_test

import (
	"context"
	"testing"
	"time"

	appnotification "github.com/eveyproject/evey/internal/app/notification"
	libevent "github.com/eveyproject/evey/internal/domain/library"
	notifdom "github.com/eveyproject/evey/internal/domain/notification"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A held prune pages the admins only: the viewers see nothing wrong yet, the
// operator must decide whether the files are really gone.
func TestNotificationService_PruneSuspended_RoutesAdminOnly(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	adminProfile := &user.Profile{ID: "p-admin", UserID: "u-admin", Name: "Root"}
	userProfile := &user.Profile{ID: "p-user", UserID: "u-user", Name: "Bob"}
	_ = profileRepo.Create(ctx, adminProfile)
	_ = profileRepo.Create(ctx, userProfile)

	svc := appnotification.NewCommandService(repo, notiffakes.NewFakeSettingsRepository(), notiffakes.NewFakeWatchHistoryAnalyzer(), profileRepo, bus,
		appnotification.WithAdminLister(&fakeAdminLister{profiles: []*user.Profile{adminProfile}}))
	svc.Register()

	bus.Publish(ctx, libevent.PruneSuspended{
		LibraryID: "lib-1", Root: "/pool", Missing: 30, Total: 90, OccurredAt: time.Now(),
	})

	adminNotifs, err := repo.ListByProfile(ctx, "p-admin", false, 0, 0)
	require.NoError(t, err)
	require.Len(t, adminNotifs, 1)
	assert.Equal(t, notifdom.TypePruneSuspended, adminNotifs[0].Type)
	assert.True(t, adminNotifs[0].Type.IsAdminOnly())
	assert.Contains(t, adminNotifs[0].Message, "30")
	assert.Contains(t, adminNotifs[0].Message, "/pool")
	assert.Contains(t, adminNotifs[0].Message, "deletion suspended")
	userNotifs, err := repo.ListByProfile(ctx, "p-user", false, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, userNotifs)
}
