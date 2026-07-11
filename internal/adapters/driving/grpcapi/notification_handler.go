package grpcapi

import (
	"context"
	"fmt"
	"slices"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	limit, offset := clampLimitOffset(req.Limit, req.Offset, 200)
	items, err := h.query.List(ctx, profileID, req.UnreadOnly, limit, offset)
	if err != nil {
		return nil, internalError("list notifications", err)
	}
	// Filter notifications by library access for non-admin users. Batch-resolve
	// the referenced media items' library IDs in one query instead of one
	// GetMediaItem per notification (N+1) - the per-item loop also risked
	// returning silent partial results if the context timed out mid-loop.
	allowed, err := h.allowedLibraryIDs(ctx)
	if err != nil {
		return nil, internalError("list notifications: library access", err)
	}
	libByMedia := map[string]string{}
	if allowed != nil {
		seen := map[string]bool{}
		ids := make([]string, 0, len(items))
		for _, n := range items {
			id := string(n.MediaItemID)
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			mis, miErr := h.libQuery.GetMediaItemsByIDs(ctx, ids)
			if miErr != nil {
				return nil, internalError("resolve notification media", miErr)
			}
			for _, mi := range mis {
				libByMedia[string(mi.ID)] = string(mi.LibraryID)
			}
		}
	}

	resp := &eveyv1.ListNotificationsResponse{}
	for _, n := range items {
		if n.MediaItemID != "" && allowed != nil {
			// Unknown id (item deleted) or a library the caller can't access → drop.
			lib, ok := libByMedia[string(n.MediaItemID)]
			if !ok || !containsStr(allowed, lib) {
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

// ListAdminNotifications returns the admin-only notifications (plugin_error,
// backup_failed) addressed to the calling admin's profile. RBAC gating to
// RoleAdmin is wired in the interceptor (see rbac.go); we still defensively
// check claims.IsAdmin so this handler is safe even if the interceptor map
// drifts.
func (h *notificationHandler) ListAdminNotifications(ctx context.Context, req *eveyv1.ListAdminNotificationsRequest) (*eveyv1.ListAdminNotificationsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.notification.ListAdminNotifications")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("notificationHandler.ListAdminNotifications: %w", err)
	}
	if !claims.IsAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "admin role required")
	}
	adminLimit, adminOffset := clampLimitOffset(req.Limit, req.Offset, 200)
	items, err := h.query.ListAdminOnly(ctx, claims.ProfileID, req.UnreadOnly, adminLimit, adminOffset)
	if err != nil {
		return nil, internalError("list admin notifications", err)
	}
	resp := &eveyv1.ListAdminNotificationsResponse{}
	for _, n := range items {
		resp.Notifications = append(resp.Notifications, notificationToProto(n))
	}
	return resp, nil
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

// allowedLibraryIDs returns the caller's accessible library IDs, or nil for an
// unrestricted (admin / no-access-service) context. A non-nil error means the
// access lookup failed and the caller MUST fail closed - returning nil on error
// would be read as "unrestricted" and leak other libraries' notifications.
func (h *notificationHandler) allowedLibraryIDs(ctx context.Context) ([]string, error) {
	if h.userAccess == nil {
		return nil, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil // zero grants = unrestricted (opt-out default)
	}
	return ids, nil
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
