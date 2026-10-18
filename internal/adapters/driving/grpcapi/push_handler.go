package grpcapi

import (
	"context"
	"encoding/base64"
	"log/slog"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appnotification "github.com/eveyproject/evey/internal/app/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// isBase64 reports whether s decodes as non-empty base64 (URL-safe or std, padded or not).
func isBase64(s string) bool {
	if s == "" {
		return false
	}
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.StdEncoding, base64.RawStdEncoding} {
		if b, err := enc.DecodeString(s); err == nil && len(b) > 0 {
			return true
		}
	}
	return false
}

type webPushHandler struct {
	eveyv1.UnimplementedPushServiceServer
	notifCmd    *appnotification.CommandService
	notifQuery  *appnotification.QueryService
	vapidPubKey string
	// registerRL caps subscriptions per user: each one is a row and a target
	// of every later notification. Nil = unlimited (tests).
	registerRL *ratelimit.Limiter
}

func (h *webPushHandler) GetVAPIDKey(ctx context.Context, _ *eveyv1.GetVAPIDKeyRequest) (*eveyv1.GetVAPIDKeyResponse, error) {
	_, span := tracing.StartSpan(ctx, "grpc.push.GetVAPIDKey")
	defer span.End()
	return &eveyv1.GetVAPIDKeyResponse{PublicKey: h.vapidPubKey}, nil
}

func (h *webPushHandler) RegisterWebPush(ctx context.Context, req *eveyv1.RegisterWebPushRequest) (*eveyv1.RegisterWebPushResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.push.RegisterWebPush")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if h.registerRL != nil && !h.registerRL.Allow(claims.UserID) {
		return nil, status.Error(codes.ResourceExhausted, "too many push registrations")
	}
	if req.Endpoint == "" || req.Keys == nil || req.Keys.P256Dh == "" || req.Keys.Auth == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint and keys required")
	}
	if !isBase64(req.Keys.P256Dh) || !isBase64(req.Keys.Auth) {
		return nil, status.Error(codes.InvalidArgument, "invalid key encoding")
	}
	if err := h.notifCmd.UpsertPushSubscription(ctx, shared.UserID(claims.UserID), shared.ProfileID(claims.ProfileID), req.Endpoint, req.Keys.P256Dh, req.Keys.Auth); err != nil {
		slog.ErrorContext(ctx, "push: register web push", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "failed to register")
	}
	return &eveyv1.RegisterWebPushResponse{Ok: true}, nil
}

func (h *webPushHandler) UnregisterWebPush(ctx context.Context, req *eveyv1.UnregisterWebPushRequest) (*eveyv1.UnregisterWebPushResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.push.UnregisterWebPush")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint required")
	}
	if err := h.notifCmd.DeletePushByEndpoint(ctx, shared.UserID(claims.UserID), req.Endpoint); err != nil {
		slog.ErrorContext(ctx, "push: unregister web push", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "failed to unregister")
	}
	return &eveyv1.UnregisterWebPushResponse{Ok: true}, nil
}

func (h *webPushHandler) ListWebPushSubscriptions(ctx context.Context, _ *eveyv1.ListWebPushSubscriptionsRequest) (*eveyv1.ListWebPushSubscriptionsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.push.ListWebPushSubscriptions")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := h.notifQuery.ListPushSubscriptions(ctx, shared.UserID(claims.UserID))
	if err != nil {
		slog.ErrorContext(ctx, "push: list subscriptions", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "failed to list")
	}
	out := make([]*eveyv1.WebPushSubscription, 0, len(subs))
	for _, s := range subs {
		out = append(out, &eveyv1.WebPushSubscription{
			Endpoint:  s.Endpoint,
			CreatedAt: timestamppb.New(s.CreatedAt),
		})
	}
	return &eveyv1.ListWebPushSubscriptionsResponse{Subscriptions: out}, nil
}

// ListDevices is deprecated (GET /api/v1/push/devices): it backed the profile
// "Devices" tab that web and mobile no longer show. There is no native
// device-token registration (the FCM-era RPCs are gone), so a user never has
// any registered native devices. It keeps answering an empty list instead of
// falling back to Unimplemented: beta builds that still open the old tab
// would otherwise get codes.Unimplemented - HTTP 501 on a settings page.
func (h *webPushHandler) ListDevices(ctx context.Context, _ *eveyv1.ListDevicesRequest) (*eveyv1.ListDevicesResponse, error) { //nolint:staticcheck // SA1019: the deprecated stub itself
	ctx, span := tracing.StartSpan(ctx, "grpc.push.ListDevices")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	return &eveyv1.ListDevicesResponse{}, nil //nolint:staticcheck // SA1019: the deprecated stub itself
}

func (h *webPushHandler) DeleteWebPushSubscription(ctx context.Context, req *eveyv1.DeleteWebPushSubscriptionRequest) (*eveyv1.DeleteWebPushSubscriptionResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.push.DeleteWebPushSubscription")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint required")
	}
	if err := h.notifCmd.DeletePushByEndpoint(ctx, shared.UserID(claims.UserID), req.Endpoint); err != nil {
		slog.ErrorContext(ctx, "push: delete subscription", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "failed to delete")
	}
	return &eveyv1.DeleteWebPushSubscriptionResponse{Ok: true}, nil
}

func (h *webPushHandler) GetPushConsent(ctx context.Context, _ *eveyv1.GetPushConsentRequest) (*eveyv1.GetPushConsentResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.push.GetPushConsent")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := h.notifQuery.ListPushSubscriptions(ctx, shared.UserID(claims.UserID))
	if err != nil {
		return nil, internalError(ctx, "push consent: list subscriptions", err)
	}
	return &eveyv1.GetPushConsentResponse{PushConsent: len(subs) > 0}, nil
}

func (h *webPushHandler) UpdatePushConsent(ctx context.Context, req *eveyv1.UpdatePushConsentRequest) (*eveyv1.UpdatePushConsentResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.push.UpdatePushConsent")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !req.Consent {
		if err := h.notifCmd.DeletePushByUser(ctx, shared.UserID(claims.UserID)); err != nil {
			slog.WarnContext(ctx, "delete push on consent revoke failed", slog.String("user_id", claims.UserID), slog.Any("err", err))
		}
	}
	return &eveyv1.UpdatePushConsentResponse{Ok: true}, nil
}
