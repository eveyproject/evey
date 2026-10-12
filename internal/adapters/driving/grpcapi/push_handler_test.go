package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appnotification "github.com/eveyproject/evey/internal/app/notification"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
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
	query := appnotification.NewQueryService(repo, settings, appnotification.WithPushSubs(subs))
	return &webPushHandler{notifCmd: svc, notifQuery: query, vapidPubKey: "PUBKEY"}
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

func TestListDevices_Unauthenticated(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	_, err := h.ListDevices(context.Background(), &eveyv1.ListDevicesRequest{})
	if got := codeOf(t, err); got != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", got)
	}
}

// ListDevices is a graceful stub for the deferred native-device feature: it
// must answer authenticated callers with an empty list (HTTP 200), NOT
// codes.Unimplemented (HTTP 501), so the profile Devices tab never 5xxes.
func TestListDevices_AuthenticatedReturnsEmpty(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	ctx := ctxWithClaims("u1", "p1", false)
	resp, err := h.ListDevices(ctx, &eveyv1.ListDevicesRequest{})
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(resp.Devices) != 0 {
		t.Errorf("Devices = %d, want 0 (empty stub)", len(resp.Devices))
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
		Keys:     &eveyv1.WebPushKeys{P256Dh: "p256", Auth: "auth"},
	})
	if got := codeOf(t, err); got != codes.Internal {
		t.Errorf("code = %v, want Internal", got)
	}
}

func TestRegisterWebPush_InvalidP256Dh(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://x",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "not-base64!!!", Auth: "auth"},
	})
	if got := codeOf(t, err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestRegisterWebPush_InvalidAuth(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	ctx := ctxWithClaims("u1", "p1", false)
	_, err := h.RegisterWebPush(ctx, &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://x",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "p256", Auth: "!!invalid"},
	})
	if got := codeOf(t, err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestIsBase64(t *testing.T) {
	t.Parallel()
	if !isBase64("p256") || !isBase64("auth") {
		t.Error("valid base64 rejected")
	}
	if isBase64("") || isBase64("not base64!!!") {
		t.Error("invalid base64 accepted")
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

// IDOR regression: UnregisterWebPush shares DeletePushByEndpoint with
// DeleteWebPushSubscription; make sure a cross-user endpoint call on it
// stays a no-op too. Relies on the repo's `WHERE user_id AND endpoint`
// filter - if someone rewrites it to endpoint-only, this catches it.
func TestUnregisterWebPush_OwnershipScopedByUserID(t *testing.T) {
	t.Parallel()
	subs := notiffakes.NewFakePushSubscriptionRepository()
	_ = subs.Upsert(context.Background(), "owner", "pOwner", "https://shared-unreg", "p", "a")
	h := newTestPushHandler(subs)

	ctx := ctxWithClaims("attacker", "pAtt", false)
	resp, err := h.UnregisterWebPush(ctx, &eveyv1.UnregisterWebPushRequest{Endpoint: "https://shared-unreg"})
	if err != nil {
		t.Fatalf("UnregisterWebPush: %v", err)
	}
	if !resp.Ok {
		t.Error("expected Ok=true (no-op for non-owner)")
	}
	list, _ := subs.ListByUser(context.Background(), "owner")
	if len(list) != 1 {
		t.Errorf("owner's subscription wrongly unregistered by another user: %+v", list)
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

// Each registration is a row and a target of every later notification: past
// the per-user budget the call is refused, and another user is not charged.
func TestRegisterWebPush_RateLimitedPerUser(t *testing.T) {
	t.Parallel()
	h := newTestPushHandler(notiffakes.NewFakePushSubscriptionRepository())
	h.registerRL = ratelimit.New(2)
	t.Cleanup(h.registerRL.Stop)
	req := &eveyv1.RegisterWebPushRequest{
		Endpoint: "https://push.example/sub",
		Keys:     &eveyv1.WebPushKeys{P256Dh: "YWJj", Auth: "ZGVm"},
	}
	ctx := ctxWithClaims("u1", "p1", false)
	for i := range 2 {
		if _, err := h.RegisterWebPush(ctx, req); err != nil {
			t.Fatalf("registration %d: %v", i+1, err)
		}
	}
	if got := codeOf(t, func() error { _, err := h.RegisterWebPush(ctx, req); return err }()); got != codes.ResourceExhausted {
		t.Fatalf("third registration: code = %v, want ResourceExhausted", got)
	}
	if _, err := h.RegisterWebPush(ctxWithClaims("u2", "p2", false), req); err != nil {
		t.Fatalf("another user must keep its own budget: %v", err)
	}
}
