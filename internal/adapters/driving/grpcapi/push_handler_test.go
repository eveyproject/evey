package grpcapi

import (
	"encoding/base64"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newWebPushHandler() *webPushHandler {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profiles := userfakes.NewFakeProfileRepository()
	events := sharedfakes.NewFakeEventBus()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	cmd := appnotification.NewCommandService(repo, settings, analyzer, profiles, events, appnotification.WithPush(nil, subs))
	return &webPushHandler{notifCmd: cmd, vapidPubKey: "pub"}
}

func TestPush_RegisterWebPush_InvalidP256Dh(t *testing.T) {
	h := newWebPushHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	validAuth := base64.RawURLEncoding.EncodeToString([]byte("auth-secret"))
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example/abc",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "not-base64!!!", Auth: validAuth},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPush_RegisterWebPush_InvalidAuth(t *testing.T) {
	h := newWebPushHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	validP256 := base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example/abc",
		Keys:     &eveyv1.WebPushKeys{P256Dh: validP256, Auth: "!!invalid"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPush_RegisterWebPush_Success(t *testing.T) {
	h := newWebPushHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	p256 := base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	auth := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	resp, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example/abc",
		Keys:     &eveyv1.WebPushKeys{P256Dh: p256, Auth: auth},
	})
	require.NoError(t, err)
	assert.True(t, resp.Ok)
}

func TestIsBase64(t *testing.T) {
	assert.True(t, isBase64(base64.RawURLEncoding.EncodeToString([]byte("hello"))))
	assert.True(t, isBase64(base64.StdEncoding.EncodeToString([]byte("hello"))))
	assert.False(t, isBase64(""))
	assert.False(t, isBase64("not base64!!!"))
}
