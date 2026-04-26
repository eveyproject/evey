package grpcapi

import (
	"context"
	"fmt"
	"slices"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type notificationHandler struct {
	eveyv1.UnimplementedNotificationServiceServer
	cmd        *appnotification.CommandService
	query      *appnotification.QueryService
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
}

func (h *notificationHandler) ListNotifications(ctx context.Context, req *eveyv1.ListNotificationsRequest) (*eveyv1.ListNotificationsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.ListNotifications")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.ListNotifications: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.ListNotifications: %w", err)
	}
	items, err := h.query.List(ctx, profileID, req.UnreadOnly, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, internalError("list notifications", err)
	}
	// Filter notifications by library access for non-admin users
	allowed := h.allowedLibraryIDs(ctx)
	resp := &eveyv1.ListNotificationsResponse{}
	for _, n := range items {
		if n.MediaItemID != "" && allowed != nil {
			item, itemErr := h.libQuery.GetMediaItem(ctx, string(n.MediaItemID))
			if itemErr != nil || !containsStr(allowed, string(item.LibraryID)) {
				continue
			}
		}
		resp.Notifications = append(resp.Notifications, notificationToProto(n))
	}
	return resp, nil
}

func (h *notificationHandler) MarkRead(ctx context.Context, req *eveyv1.MarkReadRequest) (*eveyv1.MarkReadResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.MarkRead")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.MarkRead: %w", err)
	}
	if err := h.cmd.MarkRead(ctx, req.Id, claims.ProfileID); err != nil {
		return nil, internalError("mark read", err)
	}
	return &eveyv1.MarkReadResponse{}, nil
}

func (h *notificationHandler) MarkAllRead(ctx context.Context, req *eveyv1.MarkAllReadRequest) (*eveyv1.MarkAllReadResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.MarkAllRead")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.MarkAllRead: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.MarkAllRead: %w", err)
	}
	if err := h.cmd.MarkAllRead(ctx, profileID); err != nil {
		return nil, internalError("mark all read", err)
	}
	return &eveyv1.MarkAllReadResponse{}, nil
}

func (h *notificationHandler) GetUnreadCount(ctx context.Context, req *eveyv1.GetUnreadCountRequest) (*eveyv1.GetUnreadCountResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.GetUnreadCount")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.GetUnreadCount: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.GetUnreadCount: %w", err)
	}
	count, err := h.query.UnreadCount(ctx, profileID)
	if err != nil {
		return nil, internalError("unread count", err)
	}
	return &eveyv1.GetUnreadCountResponse{Count: int32(count)}, nil
}

func (h *notificationHandler) DeleteAllNotifications(ctx context.Context, req *eveyv1.DeleteAllNotificationsRequest) (*eveyv1.DeleteAllNotificationsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.DeleteAllNotifications")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.DeleteAllNotifications: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.DeleteAllNotifications: %w", err)
	}
	if err := h.cmd.DeleteAll(ctx, profileID); err != nil {
		return nil, internalError("delete all", err)
	}
	return &eveyv1.DeleteAllNotificationsResponse{}, nil
}

func (h *notificationHandler) GetNotificationSettings(ctx context.Context, req *eveyv1.GetNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.GetNotificationSettings")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.GetNotificationSettings: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.GetNotificationSettings: %w", err)
	}
	s, err := h.query.GetSettings(ctx, profileID)
	if err != nil {
		return nil, internalError("get settings", err)
	}
	return settingsToProto(s), nil
}

func (h *notificationHandler) UpdateNotificationSettings(ctx context.Context, req *eveyv1.UpdateNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.UpdateNotificationSettings")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.UpdateNotificationSettings: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.UpdateNotificationSettings: %w", err)
	}
	s := &notification.Settings{
		ProfileID:   shared.ProfileID(profileID),
		MovieLevel:  levelFromProto(req.MovieLevel),
		SeriesLevel: levelFromProto(req.SeriesLevel),
		MusicLevel:  levelFromProto(req.MusicLevel),
	}
	if err := h.cmd.UpdateSettings(ctx, s); err != nil {
		return nil, internalError("update settings", err)
	}
	return settingsToProto(s), nil
}

func (h *notificationHandler) GetGlobalNotificationSettings(ctx context.Context, _ *eveyv1.GetGlobalNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.GetGlobalNotificationSettings")
	defer span.End()
	s, err := h.query.GetGlobalSettings(ctx)
	if err != nil {
		return nil, internalError("get global settings", err)
	}
	return settingsToProto(s), nil
}

func (h *notificationHandler) UpdateGlobalNotificationSettings(ctx context.Context, req *eveyv1.UpdateGlobalNotificationSettingsRequest) (*eveyv1.NotificationSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.UpdateGlobalNotificationSettings")
	defer span.End()
	s := &notification.Settings{
		ProfileID:   "",
		MovieLevel:  levelFromProto(req.MovieLevel),
		SeriesLevel: levelFromProto(req.SeriesLevel),
		MusicLevel:  levelFromProto(req.MusicLevel),
	}
	if err := h.cmd.UpdateGlobalSettings(ctx, s); err != nil {
		return nil, internalError("update global settings", err)
	}
	return settingsToProto(s), nil
}

func (h *notificationHandler) allowedLibraryIDs(ctx context.Context) []string {
	if h.userAccess == nil {
		return nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil
	}
	ids, _ := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	return ids
}

func containsStr(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

func notificationToProto(n *notification.Notification) *eveyv1.Notification {
	return &eveyv1.Notification{
		Id:          n.ID,
		ProfileId:   string(n.ProfileID),
		Type:        string(n.Type),
		Title:       n.Title,
		Message:     n.Message,
		MediaItemId: string(n.MediaItemID),
		Level:       string(n.Level),
		CreatedAt:   toTimestamppb(n.CreatedAt),
		ExpiresAt:   toTimestamppbPtr(n.ExpiresAt),
		ReadAt:      toTimestamppbPtr(n.ReadAt),
	}
}

func settingsToProto(s *notification.Settings) *eveyv1.NotificationSettings {
	return &eveyv1.NotificationSettings{
		ProfileId:   string(s.ProfileID),
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
