package playback_test

import (
	"context"
	"testing"

	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	"github.com/gravity-zero/evey/internal/domain/playback"
	pbfakes "github.com/gravity-zero/evey/internal/domain/playback/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
)

func TestStartStream_MaxStreams(t *testing.T) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	bus := sharedfakes.NewFakeEventBus()

	maxStreams := 1
	userRepo.Create(context.Background(), &user.User{
		ID:         "u1",
		Username:   "test",
		Enabled:    true,
		MaxStreams: &maxStreams,
	})

	svc := appplayback.NewService(progressRepo, sessionRepo, userRepo, bus)
	ctx := context.Background()

	err := svc.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	assert.NoError(t, err)

	err = svc.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrMaxStreams)
}
