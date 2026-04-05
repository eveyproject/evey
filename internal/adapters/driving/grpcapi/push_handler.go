package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type webPushHandler struct {
	eveyv1.UnimplementedPushServiceServer
	subs        notification.PushSubscriptionRepository
	profiles    user.ProfileRepository
	users       user.UserRepository
	vapidPubKey string
}

func (h *webPushHandler) GetVAPIDKey(_ context.Context, _ *eveyv1.GetVAPIDKeyRequest) (*eveyv1.GetVAPIDKeyResponse, error) {
	return &eveyv1.GetVAPIDKeyResponse{PublicKey: h.vapidPubKey}, nil
}

func (h *webPushHandler) RegisterWebPush(ctx context.Context, req *eveyv1.RegisterWebPushRequest) (*eveyv1.RegisterWebPushResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if req.Endpoint == "" || req.Keys == nil || req.Keys.P256Dh == "" || req.Keys.Auth == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint and keys required")
	}
	if err := h.subs.Upsert(ctx, claims.UserID, claims.ProfileID, req.Endpoint, req.Keys.P256Dh, req.Keys.Auth); err != nil {
		slog.Error("push: register web push", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "failed to register")
	}
	return &eveyv1.RegisterWebPushResponse{Ok: true}, nil
}

func (h *webPushHandler) UnregisterWebPush(ctx context.Context, req *eveyv1.UnregisterWebPushRequest) (*eveyv1.UnregisterWebPushResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint required")
	}
	if err := h.subs.DeleteByEndpoint(ctx, claims.UserID, req.Endpoint); err != nil {
		slog.Error("push: unregister web push", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "failed to unregister")
	}
	return &eveyv1.UnregisterWebPushResponse{Ok: true}, nil
}

func (h *webPushHandler) ListWebPushSubscriptions(ctx context.Context, _ *eveyv1.ListWebPushSubscriptionsRequest) (*eveyv1.ListWebPushSubscriptionsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := h.subs.ListByUser(ctx, claims.UserID)
	if err != nil {
		slog.Error("push: list subscriptions", slog.Any("err", err))
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

func (h *webPushHandler) DeleteWebPushSubscription(ctx context.Context, req *eveyv1.DeleteWebPushSubscriptionRequest) (*eveyv1.DeleteWebPushSubscriptionResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint required")
	}
	if err := h.subs.DeleteByEndpoint(ctx, claims.UserID, req.Endpoint); err != nil {
		slog.Error("push: delete subscription", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "failed to delete")
	}
	return &eveyv1.DeleteWebPushSubscriptionResponse{Ok: true}, nil
}

func (h *webPushHandler) GetPushConsent(ctx context.Context, _ *eveyv1.GetPushConsentRequest) (*eveyv1.GetPushConsentResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := h.subs.ListByUser(ctx, claims.UserID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to check consent")
	}
	return &eveyv1.GetPushConsentResponse{PushConsent: len(subs) > 0}, nil
}

func (h *webPushHandler) UpdatePushConsent(ctx context.Context, req *eveyv1.UpdatePushConsentRequest) (*eveyv1.UpdatePushConsentResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !req.Consent {
		_ = h.subs.DeleteByUser(ctx, claims.UserID)
	}
	return &eveyv1.UpdatePushConsentResponse{Ok: true}, nil
}
