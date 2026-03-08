package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type notificationHandler struct {
	eveyv1.UnimplementedNotificationServiceServer
	svc *appnotification.Service
}

func (h *notificationHandler) ListNotifications(ctx context.Context, req *eveyv1.ListNotificationsRequest) (*eveyv1.ListNotificationsResponse, error) {
	items, err := h.svc.List(ctx, req.ProfileId, req.UnreadOnly, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list notifications: %v", err)
	}
	resp := &eveyv1.ListNotificationsResponse{}
	for _, n := range items {
		resp.Notifications = append(resp.Notifications, notificationToProto(n))
	}
	return resp, nil
}

func (h *notificationHandler) MarkRead(ctx context.Context, req *eveyv1.MarkReadRequest) (*eveyv1.MarkReadResponse, error) {
	if err := h.svc.MarkRead(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "mark read: %v", err)
	}
	return &eveyv1.MarkReadResponse{}, nil
}

func (h *notificationHandler) MarkAllRead(ctx context.Context, req *eveyv1.MarkAllReadRequest) (*eveyv1.MarkAllReadResponse, error) {
	if err := h.svc.MarkAllRead(ctx, req.ProfileId); err != nil {
		return nil, status.Errorf(codes.Internal, "mark all read: %v", err)
	}
	return &eveyv1.MarkAllReadResponse{}, nil
}

func (h *notificationHandler) GetUnreadCount(ctx context.Context, req *eveyv1.GetUnreadCountRequest) (*eveyv1.GetUnreadCountResponse, error) {
	count, err := h.svc.UnreadCount(ctx, req.ProfileId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unread count: %v", err)
	}
	return &eveyv1.GetUnreadCountResponse{Count: int32(count)}, nil
}

func (h *notificationHandler) DeleteAllNotifications(ctx context.Context, req *eveyv1.DeleteAllNotificationsRequest) (*eveyv1.DeleteAllNotificationsResponse, error) {
	if err := h.svc.DeleteAll(ctx, req.ProfileId); err != nil {
		return nil, status.Errorf(codes.Internal, "delete all: %v", err)
	}
	return &eveyv1.DeleteAllNotificationsResponse{}, nil
}

func (h *notificationHandler) GetNotificationSettings(ctx context.Context, req *eveyv1.GetNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	s, err := h.svc.GetSettings(ctx, req.ProfileId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get settings: %v", err)
	}
	return settingsToProto(s), nil
}

func (h *notificationHandler) UpdateNotificationSettings(ctx context.Context, req *eveyv1.UpdateNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	s := &notification.Settings{
		ProfileID:   req.ProfileId,
		MovieLevel:  levelFromProto(req.MovieLevel),
		SeriesLevel: levelFromProto(req.SeriesLevel),
		MusicLevel:  levelFromProto(req.MusicLevel),
	}
	if err := h.svc.UpdateSettings(ctx, s); err != nil {
		return nil, status.Errorf(codes.Internal, "update settings: %v", err)
	}
	return settingsToProto(s), nil
}

func (h *notificationHandler) GetGlobalNotificationSettings(ctx context.Context, _ *eveyv1.GetGlobalNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	s, err := h.svc.GetGlobalSettings(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get global settings: %v", err)
	}
	return settingsToProto(s), nil
}

func (h *notificationHandler) UpdateGlobalNotificationSettings(ctx context.Context, req *eveyv1.UpdateGlobalNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	s := &notification.Settings{
		ProfileID:   "",
		MovieLevel:  levelFromProto(req.MovieLevel),
		SeriesLevel: levelFromProto(req.SeriesLevel),
		MusicLevel:  levelFromProto(req.MusicLevel),
	}
	if err := h.svc.UpdateGlobalSettings(ctx, s); err != nil {
		return nil, status.Errorf(codes.Internal, "update global settings: %v", err)
	}
	return settingsToProto(s), nil
}

func notificationToProto(n *notification.Notification) *eveyv1.Notification {
	return &eveyv1.Notification{
		Id:          n.ID,
		ProfileId:   n.ProfileID,
		Type:        string(n.Type),
		Title:       n.Title,
		Message:     n.Message,
		MediaItemId: n.MediaItemID,
		Level:       string(n.Level),
		CreatedAt:   toTimestamppb(n.CreatedAt),
		ExpiresAt:   toTimestamppbPtr(n.ExpiresAt),
		ReadAt:      toTimestamppbPtr(n.ReadAt),
	}
}

func settingsToProto(s *notification.Settings) *eveyv1.NotificationSettings {
	return &eveyv1.NotificationSettings{
		ProfileId:   s.ProfileID,
		MovieLevel:  levelToProto(s.MovieLevel),
		SeriesLevel: levelToProto(s.SeriesLevel),
		MusicLevel:  levelToProto(s.MusicLevel),
	}
}

func levelToProto(l notification.Level) eveyv1.NotificationLevel {
	switch l {
	case notification.LevelDisabled:
		return eveyv1.NotificationLevel_NOTIFICATION_LEVEL_DISABLED
	case notification.LevelAll:
		return eveyv1.NotificationLevel_NOTIFICATION_LEVEL_ALL
	case notification.LevelRelevant:
		return eveyv1.NotificationLevel_NOTIFICATION_LEVEL_RELEVANT
	case notification.LevelHighlyRelevant:
		return eveyv1.NotificationLevel_NOTIFICATION_LEVEL_HIGHLY_RELEVANT
	default:
		return eveyv1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED
	}
}

func levelFromProto(l eveyv1.NotificationLevel) notification.Level {
	switch l {
	case eveyv1.NotificationLevel_NOTIFICATION_LEVEL_DISABLED:
		return notification.LevelDisabled
	case eveyv1.NotificationLevel_NOTIFICATION_LEVEL_ALL:
		return notification.LevelAll
	case eveyv1.NotificationLevel_NOTIFICATION_LEVEL_RELEVANT:
		return notification.LevelRelevant
	case eveyv1.NotificationLevel_NOTIFICATION_LEVEL_HIGHLY_RELEVANT:
		return notification.LevelHighlyRelevant
	default:
		return notification.LevelAll
	}
}
