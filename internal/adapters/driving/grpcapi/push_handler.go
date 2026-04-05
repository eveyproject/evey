package grpcapi

import (
	"context"
	"fmt"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type webPushHandler struct {
	eveyv1.UnimplementedPushServiceServer
	subs        notification.PushSubscriptionRepository
	profiles    user.ProfileRepository
	users       user.UserRepository
	vapidPubKey string
	registerRL  *ratelimit.Limiter
}

func (h *webPushHandler) GetVAPIDKey(_ context.Context, _ *eveyv1.GetVAPIDKeyRequest) (*eveyv1.GetVAPIDKeyResponse, error) {
	return &eveyv1.GetVAPIDKeyResponse{PublicKey: h.vapidPubKey}, nil
}

func (h *webPushHandler) RegisterWebPush(ctx context.Context, req *eveyv1.RegisterWebPushRequest) (*eveyv1.RegisterWebPushResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("webPushHandler.RegisterWebPush: %w", err)
	}
	if !h.registerRL.Allow(claims.UserID) {
		return nil, status.Error(codes.ResourceExhausted, "too many requests")
	}
	if req.Endpoint == "" || req.Keys == nil || req.Keys.P256Dh == "" || req.Keys.Auth == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid body")
	}
	if !req.Consent {
		return nil, status.Error(codes.InvalidArgument, "explicit consent required for push notifications")
	}
	if claims.ProfileID != "" {
		profile, pErr := h.profiles.GetByID(ctx, claims.ProfileID)
		if pErr != nil || profile.UserID != claims.UserID {
			slog.Warn("push register: profile ownership mismatch", slog.String("user_id", claims.UserID), slog.String("profile_id", claims.ProfileID))
			return nil, status.Error(codes.PermissionDenied, "forbidden")
		}
	}
	usr, err := h.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get user: %v", err)
	}
	if !usr.PushConsent {
		usr.PushConsent = true
		if err := h.users.Update(ctx, usr); err != nil {
			slog.Warn("push register: persist consent failed", slog.Any("err", err))
			return nil, status.Errorf(codes.Internal, "persist consent: %v", err)
		}
		slog.Info("push consent persisted", slog.String("user_id", claims.UserID))
	}
	if err := h.subs.Upsert(ctx, claims.UserID, claims.ProfileID, req.Endpoint, req.Keys.P256Dh, req.Keys.Auth); err != nil {
		slog.Warn("push register: upsert failed", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "upsert: %v", err)
	}
	return &eveyv1.RegisterWebPushResponse{Ok: true}, nil
}

func (h *webPushHandler) UnregisterWebPush(ctx context.Context, req *eveyv1.UnregisterWebPushRequest) (*eveyv1.UnregisterWebPushResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("webPushHandler.UnregisterWebPush: %w", err)
	}
	if req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid body")
	}
	if err := h.subs.DeleteByEndpoint(ctx, claims.UserID, req.Endpoint); err != nil {
		slog.Warn("push unregister: delete failed", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "delete: %v", err)
	}
	return &eveyv1.UnregisterWebPushResponse{Ok: true}, nil
}

func (h *webPushHandler) ListWebPushSubscriptions(ctx context.Context, _ *eveyv1.ListWebPushSubscriptionsRequest) (*eveyv1.ListWebPushSubscriptionsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("webPushHandler.ListWebPushSubscriptions: %w", err)
	}
	subs, err := h.subs.ListByUser(ctx, claims.UserID)
	if err != nil {
		slog.Warn("push subscriptions: list failed", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "list: %v", err)
	}
	resp := &eveyv1.ListWebPushSubscriptionsResponse{}
	for _, s := range subs {
		resp.Subscriptions = append(resp.Subscriptions, &eveyv1.WebPushSubscription{
			Endpoint:  s.Endpoint,
			CreatedAt: toTimestamppb(s.CreatedAt),
		})
	}
	return resp, nil
}

func (h *webPushHandler) DeleteWebPushSubscription(ctx context.Context, req *eveyv1.DeleteWebPushSubscriptionRequest) (*eveyv1.DeleteWebPushSubscriptionResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("webPushHandler.DeleteWebPushSubscription: %w", err)
	}
	if req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid body")
	}
	if err := h.subs.DeleteByEndpoint(ctx, claims.UserID, req.Endpoint); err != nil {
		slog.Warn("push subscriptions: delete failed", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "delete: %v", err)
	}
	return &eveyv1.DeleteWebPushSubscriptionResponse{Ok: true}, nil
}

func (h *webPushHandler) GetPushConsent(ctx context.Context, _ *eveyv1.GetPushConsentRequest) (*eveyv1.GetPushConsentResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("webPushHandler.GetPushConsent: %w", err)
	}
	usr, err := h.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get user: %v", err)
	}
	return &eveyv1.GetPushConsentResponse{PushConsent: usr.PushConsent}, nil
}

func (h *webPushHandler) UpdatePushConsent(ctx context.Context, req *eveyv1.UpdatePushConsentRequest) (*eveyv1.UpdatePushConsentResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("webPushHandler.UpdatePushConsent: %w", err)
	}
	usr, err := h.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get user: %v", err)
	}
	usr.PushConsent = req.Consent
	if err := h.users.Update(ctx, usr); err != nil {
		slog.Warn("push consent: update failed", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "update: %v", err)
	}
	if !req.Consent {
		if err := h.subs.DeleteByUser(ctx, claims.UserID); err != nil {
			slog.Warn("push consent: delete subscriptions failed", slog.Any("err", err))
			return nil, status.Errorf(codes.Internal, "delete subscriptions: %v", err)
		}
		slog.Info("push consent revoked, subscriptions deleted", slog.String("user_id", claims.UserID))
	} else {
		slog.Info("push consent granted", slog.String("user_id", claims.UserID))
	}
	return &eveyv1.UpdatePushConsentResponse{Ok: true}, nil
}
