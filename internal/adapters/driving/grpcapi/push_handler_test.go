package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestPushHandler(subs *notiffakes.FakePushSubscriptionRepository) *webPushHandler {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profiles := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	sender := notiffakes.NewFakePushSender()
	svc := appnotification.NewCommandService(repo, settings, analyzer, profiles, bus,
		appnotification.WithPush(sender, subs),
	)
	return &webPushHandler{notifCmd: svc, vapidPubKey: "PUBKEY"}
}

func codeOf(t *testing.T, err error) codes.Code {
	t.Helper()
	if err == nil {
		return codes.OK
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a gRPC status: %v", err)
	}
	return s.Code()
}

func TestGetVAPIDKey_ReturnsPublicKey(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	resp, err := h.GetVAPIDKey(context.Background(), &eveyv1.GetVAPIDKeyRequest{})
	if err != nil {
		t.Fatalf("GetVAPIDKey: %v", err)
	}
	if resp.PublicKey != "PUBKEY" {
		t.Errorf("PublicKey = %q, want PUBKEY", resp.PublicKey)
	}
}

func TestRegisterWebPush_Unauthenticated(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	_, err := h.RegisterWebPush(context.Background(), &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://fcm.googleapis.com/x",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "a", Auth: "b"},
	})
	if got := codeOf(t, err); got != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", got)
	}
}

func TestRegisterWebPush_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "a", Auth: "b"},
	})
	if got := codeOf(t, err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestRegisterWebPush_MissingKeys(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	ctx := ctxWithClaims("u1", "p1", false)

	cases := []struct {
		name string
		req  *eveyv1.RegisterWebPushRequest
	}{
		{"nil keys", &eveyv1.RegisterWebPushRequest{Endpoint: "https://x", Keys: nil}},
		{"empty p256dh", &eveyv1.RegisterWebPushRequest{Endpoint: "https://x", Keys: &eveyv1.WebPushKeys{P256Dh: "", Auth: "b"}}},
		{"empty auth", &eveyv1.RegisterWebPushRequest{Endpoint: "https://x", Keys: &eveyv1.WebPushKeys{P256Dh: "a", Auth: ""}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := h.RegisterWebPush(ctx, tc.req)
			if got := codeOf(t, err); got != codes.InvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", got)
			}
		})
	}
}

func TestRegisterWebPush_HappyPath_PersistsWithCallerUserID(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	h := newTestPushHandler(subs)
	ctx := ctxWithClaims("u1", "p1", false)

	resp, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://fcm.googleapis.com/abc",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "p256", Auth: "auth"},
	})
	if err != nil {
		t.Fatalf("RegisterWebPush: %v", err)
	}
	if !resp.Ok {
		t.Error("expected Ok=true")
	}
	list, _ := subs.ListByUser(context.Background(), "u1")
	if len(list) != 1 || list[0].Endpoint != "https://fcm.googleapis.com/abc" {
		t.Errorf("subscription not persisted: %+v", list)
	}
}

func TestRegisterWebPush_RepoError_Internal(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	subs.Err = errors.New("db boom")
	h := newTestPushHandler(subs)
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://x",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "a", Auth: "b"},
	})
	if got := codeOf(t, err); got != codes.Internal {
		t.Errorf("code = %v, want Internal", got)
	}
}

func TestUnregisterWebPush_Unauthenticated(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	_, err := h.UnregisterWebPush(context.Background(), &eveyv1.UnregisterWebPushRequest{Endpoint: "https://x"})
	if got := codeOf(t, err); got != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", got)
	}
}

func TestUnregisterWebPush_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.UnregisterWebPush(ctx, &eveyv1.UnregisterWebPushRequest{Endpoint: ""})
	if got := codeOf(t, err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestDeleteWebPushSubscription_OwnershipScopedByUserID(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	_ = subs.Upsert(context.Background(), "owner", "pOwner", "https://shared", "p", "a")
	h := newTestPushHandler(subs)

	// Attacker with different userID tries to delete owner's subscription.
	ctx := ctxWithClaims("attacker", "pAtt", false)
	resp, err := h.DeleteWebPushSubscription(ctx, &eveyv1.DeleteWebPushSubscriptionRequest{Endpoint: "https://shared"})
	if err != nil {
		t.Fatalf("DeleteWebPushSubscription: %v", err)
	}
	if !resp.Ok {
		t.Error("expected Ok=true (no-op for non-owner)")
	}
	// Subscription must still exist.
	list, _ := subs.ListByUser(context.Background(), "owner")
	if len(list) != 1 {
		t.Errorf("owner's subscription wrongly deleted by another user: %+v", list)
	}
}

func TestDeleteWebPushSubscription_OwnerCanDelete(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	_ = subs.Upsert(context.Background(), "u1", "p1", "https://mine", "p", "a")
	h := newTestPushHandler(subs)
	ctx := ctxWithClaims("u1", "p1", false)

	_, err := h.DeleteWebPushSubscription(ctx, &eveyv1.DeleteWebPushSubscriptionRequest{Endpoint: "https://mine"})
	if err != nil {
		t.Fatalf("DeleteWebPushSubscription: %v", err)
	}
	list, _ := subs.ListByUser(context.Background(), "u1")
	if len(list) != 0 {
		t.Errorf("expected subscription removed, got %+v", list)
	}
}

func TestDeleteWebPushSubscription_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.DeleteWebPushSubscription(ctx, &eveyv1.DeleteWebPushSubscriptionRequest{Endpoint: ""})
	if got := codeOf(t, err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestListWebPushSubscriptions_ScopedByUser(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	_ = subs.Upsert(context.Background(), "u1", "p1", "https://a", "p", "a")
	_ = subs.Upsert(context.Background(), "u2", "p2", "https://b", "p", "a")
	h := newTestPushHandler(subs)
	ctx := ctxWithClaims("u1", "p1", false)

	resp, err := h.ListWebPushSubscriptions(ctx, &eveyv1.ListWebPushSubscriptionsRequest{})
	if err != nil {
		t.Fatalf("ListWebPushSubscriptions: %v", err)
	}
	if len(resp.Subscriptions) != 1 || resp.Subscriptions[0].Endpoint != "https://a" {
		t.Errorf("expected only u1's subscription, got %+v", resp.Subscriptions)
	}
}

func TestGetPushConsent_TrueWhenSubsExist(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	_ = subs.Upsert(context.Background(), "u1", "p1", "https://a", "p", "a")
	h := newTestPushHandler(subs)
	ctx := ctxWithClaims("u1", "p1", false)

	resp, err := h.GetPushConsent(ctx, &eveyv1.GetPushConsentRequest{})
	if err != nil {
		t.Fatalf("GetPushConsent: %v", err)
	}
	if !resp.PushConsent {
		t.Error("expected consent=true")
	}
}

func TestUpdatePushConsent_RevokeDeletesAll(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	_ = subs.Upsert(context.Background(), "u1", "p1", "https://a", "p", "a")
	_ = subs.Upsert(context.Background(), "u1", "p1", "https://b", "p", "a")
	h := newTestPushHandler(subs)
	ctx := ctxWithClaims("u1", "p1", false)

	_, err := h.UpdatePushConsent(ctx, &eveyv1.UpdatePushConsentRequest{Consent: false})
	if err != nil {
		t.Fatalf("UpdatePushConsent: %v", err)
	}
	list, _ := subs.ListByUser(context.Background(), "u1")
	if len(list) != 0 {
		t.Errorf("expected all subs deleted on revoke, got %+v", list)
	}
}
