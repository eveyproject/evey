package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newPushHandler() *webPushHandler {
	return &webPushHandler{
		subs:        notiffakes.NewFakePushSubscriptionRepository(),
		profiles:    userfakes.NewFakeProfileRepository(),
		users:       userfakes.NewFakeUserRepository(),
		vapidPubKey: "test-vapid-key",
		registerRL:  ratelimit.New(10),
	}
}

func TestGetVAPIDKey(t *testing.T) {
	h := newPushHandler()
	resp, err := h.GetVAPIDKey(context.Background(), &eveyv1.GetVAPIDKeyRequest{})
	require.NoError(t, err)
	assert.Equal(t, "test-vapid-key", resp.PublicKey)
}

func TestRegisterWebPush_Unauthenticated(t *testing.T) {
	h := newPushHandler()
	_, err := h.RegisterWebPush(context.Background(), &eveyv1.RegisterWebPushRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestRegisterWebPush_InvalidBody(t *testing.T) {
	h := newPushHandler()
	ctx := ctxWithClaims("u1", "p1", false)

	cases := []struct {
		name string
		req  *eveyv1.RegisterWebPushRequest
	}{
		{"empty_endpoint", &eveyv1.RegisterWebPushRequest{Endpoint: "", Keys: &eveyv1.WebPushKeys{P256Dh: "abc", Auth: "def"}, Consent: true}},
		{"nil_keys", &eveyv1.RegisterWebPushRequest{Endpoint: "https://push.example.com/1", Keys: nil, Consent: true}},
		{"empty_p256dh", &eveyv1.RegisterWebPushRequest{Endpoint: "https://push.example.com/1", Keys: &eveyv1.WebPushKeys{P256Dh: "", Auth: "def"}, Consent: true}},
		{"empty_auth", &eveyv1.RegisterWebPushRequest{Endpoint: "https://push.example.com/1", Keys: &eveyv1.WebPushKeys{P256Dh: "abc", Auth: ""}, Consent: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.RegisterWebPush(ctx, tc.req)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestRegisterWebPush_NoConsent(t *testing.T) {
	h := newPushHandler()
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example.com/1",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "abc", Auth: "def"},
		Consent:  false,
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRegisterWebPush_ProfileOwnershipMismatch(t *testing.T) {
	h := newPushHandler()
	profileRepo := h.profiles.(*userfakes.FakeProfileRepository)
	_ = profileRepo.Create(context.Background(), &user.Profile{ID: "p1", UserID: "u-owner", Name: "Owner"})

	ctx := ctxWithClaims("u-attacker", "p1", false)
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example.com/1",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "abc", Auth: "def"},
		Consent:  true,
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRegisterWebPush_Success(t *testing.T) {
	h := newPushHandler()
	userRepo := h.users.(*userfakes.FakeUserRepository)
	_ = userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "user", Enabled: true})

	ctx := ctxWithClaims("u1", "", false)
	resp, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example.com/1",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "abc", Auth: "def"},
		Consent:  true,
	})
	require.NoError(t, err)
	assert.True(t, resp.Ok)

	usr, _ := userRepo.GetByID(context.Background(), "u1")
	assert.True(t, usr.PushConsent)
}

func TestUnregisterWebPush_Unauthenticated(t *testing.T) {
	h := newPushHandler()
	_, err := h.UnregisterWebPush(context.Background(), &eveyv1.UnregisterWebPushRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestUnregisterWebPush_EmptyEndpoint(t *testing.T) {
	h := newPushHandler()
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.UnregisterWebPush(ctx, &eveyv1.UnregisterWebPushRequest{Endpoint: ""})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUnregisterWebPush_Success(t *testing.T) {
	h := newPushHandler()
	subRepo := h.subs.(*notiffakes.FakePushSubscriptionRepository)
	_ = subRepo.Upsert(context.Background(), "u1", "p1", "https://push.example.com/1", "abc", "def")

	ctx := ctxWithClaims("u1", "p1", false)
	resp, err := h.UnregisterWebPush(ctx, &eveyv1.UnregisterWebPushRequest{Endpoint: "https://push.example.com/1"})
	require.NoError(t, err)
	assert.True(t, resp.Ok)

	subs, _ := subRepo.ListByUser(context.Background(), "u1")
	assert.Empty(t, subs)
}

func TestListWebPushSubscriptions_Unauthenticated(t *testing.T) {
	h := newPushHandler()
	_, err := h.ListWebPushSubscriptions(context.Background(), &eveyv1.ListWebPushSubscriptionsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestListWebPushSubscriptions_Success(t *testing.T) {
	h := newPushHandler()
	subRepo := h.subs.(*notiffakes.FakePushSubscriptionRepository)
	_ = subRepo.Upsert(context.Background(), "u1", "p1", "https://push.example.com/1", "abc", "def")
	_ = subRepo.Upsert(context.Background(), "u1", "p1", "https://push.example.com/2", "ghi", "jkl")

	ctx := ctxWithClaims("u1", "p1", false)
	resp, err := h.ListWebPushSubscriptions(ctx, &eveyv1.ListWebPushSubscriptionsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Subscriptions, 2)
}

func TestDeleteWebPushSubscription_EmptyEndpoint(t *testing.T) {
	h := newPushHandler()
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.DeleteWebPushSubscription(ctx, &eveyv1.DeleteWebPushSubscriptionRequest{Endpoint: ""})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestDeleteWebPushSubscription_Success(t *testing.T) {
	h := newPushHandler()
	subRepo := h.subs.(*notiffakes.FakePushSubscriptionRepository)
	_ = subRepo.Upsert(context.Background(), "u1", "p1", "https://push.example.com/1", "abc", "def")
	_ = subRepo.Upsert(context.Background(), "u1", "p1", "https://push.example.com/2", "ghi", "jkl")

	ctx := ctxWithClaims("u1", "p1", false)
	resp, err := h.DeleteWebPushSubscription(ctx, &eveyv1.DeleteWebPushSubscriptionRequest{Endpoint: "https://push.example.com/1"})
	require.NoError(t, err)
	assert.True(t, resp.Ok)

	subs, _ := subRepo.ListByUser(context.Background(), "u1")
	assert.Len(t, subs, 1)
}

func TestGetPushConsent_Unauthenticated(t *testing.T) {
	h := newPushHandler()
	_, err := h.GetPushConsent(context.Background(), &eveyv1.GetPushConsentRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGetPushConsent_Success(t *testing.T) {
	h := newPushHandler()
	userRepo := h.users.(*userfakes.FakeUserRepository)
	_ = userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "user", Enabled: true, PushConsent: true})

	ctx := ctxWithClaims("u1", "p1", false)
	resp, err := h.GetPushConsent(ctx, &eveyv1.GetPushConsentRequest{})
	require.NoError(t, err)
	assert.True(t, resp.PushConsent)
}

func TestUpdatePushConsent_Grant(t *testing.T) {
	h := newPushHandler()
	userRepo := h.users.(*userfakes.FakeUserRepository)
	_ = userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "user", Enabled: true})

	ctx := ctxWithClaims("u1", "p1", false)
	resp, err := h.UpdatePushConsent(ctx, &eveyv1.UpdatePushConsentRequest{Consent: true})
	require.NoError(t, err)
	assert.True(t, resp.Ok)

	usr, _ := userRepo.GetByID(context.Background(), "u1")
	assert.True(t, usr.PushConsent)
}

func TestUpdatePushConsent_Revoke_DeletesSubscriptions(t *testing.T) {
	h := newPushHandler()
	userRepo := h.users.(*userfakes.FakeUserRepository)
	_ = userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "user", Enabled: true, PushConsent: true})
	subRepo := h.subs.(*notiffakes.FakePushSubscriptionRepository)
	_ = subRepo.Upsert(context.Background(), "u1", "p1", "https://push.example.com/1", "abc", "def")
	_ = subRepo.Upsert(context.Background(), "u1", "p1", "https://push.example.com/2", "ghi", "jkl")

	ctx := ctxWithClaims("u1", "p1", false)
	resp, err := h.UpdatePushConsent(ctx, &eveyv1.UpdatePushConsentRequest{Consent: false})
	require.NoError(t, err)
	assert.True(t, resp.Ok)

	usr, _ := userRepo.GetByID(context.Background(), "u1")
	assert.False(t, usr.PushConsent)
	subs, _ := subRepo.ListByUser(context.Background(), "u1")
	assert.Empty(t, subs)
}

func TestRegisterWebPush_RepoError(t *testing.T) {
	h := newPushHandler()
	userRepo := h.users.(*userfakes.FakeUserRepository)
	_ = userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "user", Enabled: true})
	subRepo := h.subs.(*notiffakes.FakePushSubscriptionRepository)
	subRepo.Err = errors.New("db error")

	ctx := ctxWithClaims("u1", "", false)
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example.com/1",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "abc", Auth: "def"},
		Consent:  true,
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
