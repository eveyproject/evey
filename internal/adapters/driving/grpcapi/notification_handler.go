package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"slices"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appnotification "github.com/eveyproject/evey/internal/app/notification"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	items, err := h.query.List(ctx, shared.ProfileID(profileID), req.UnreadOnly, limit, offset)
	if err != nil {
		return nil, internalError(ctx, "list notifications", err)
	}
	// Filter notifications by library access for non-admin users. Batch-resolve
	// the referenced media items' library IDs in one query instead of one
	// GetMediaItem per notification (N+1) - the per-item loop also risked
	// returning silent partial results if the context timed out mid-loop.
	allowed, err := h.allowedLibraryIDs(ctx)
	if err != nil {
		return nil, internalError(ctx, "list notifications: library access", err)
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
				return nil, internalError(ctx, "resolve notification media", miErr)
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
	if err := h.cmd.MarkRead(ctx, req.Id, shared.ProfileID(claims.ProfileID)); err != nil {
		// The repository now distinguishes "no such notification, or not this
		// profile's" from a real failure. Naming something that does not exist
		// is the caller's mistake, not the server's - and the same answer for
		// another profile's id keeps that id unguessable.
		if errors.Is(err, shared.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "notification not found")
		}
		return nil, internalError(ctx, "mark read", err)
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
	if err := h.cmd.MarkAllRead(ctx, shared.ProfileID(profileID)); err != nil {
		return nil, internalError(ctx, "mark all read", err)
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
	count, err := h.query.UnreadCount(ctx, shared.ProfileID(profileID))
	if err != nil {
		return nil, internalError(ctx, "unread count", err)
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
	if err := h.cmd.DeleteAll(ctx, shared.ProfileID(profileID)); err != nil {
		return nil, internalError(ctx, "delete all", err)
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
	s, err := h.query.GetSettings(ctx, shared.ProfileID(profileID))
	if err != nil {
		return nil, internalError(ctx, "get settings", err)
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
		return nil, internalError(ctx, "update settings", err)
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
	items, err := h.query.ListAdminOnly(ctx, shared.ProfileID(claims.ProfileID), req.UnreadOnly, adminLimit, adminOffset)
	if err != nil {
		return nil, internalError(ctx, "list admin notifications", err)
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
		return nil, internalError(ctx, "get global settings", err)
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
		return nil, internalError(ctx, "update global settings", err)
	}
	return settingsToProto(s), nil
}

// allowedLibraryIDs returns the caller's accessible library IDs, or nil for an
// unrestricted admin context. A non-nil error means the access lookup failed
// and the caller MUST fail closed - returning nil on error would be read as
// "unrestricted" and leak other libraries' notifications. An unwired
// userAccess port also fails closed (non-nil empty slice), not unrestricted.
func (h *notificationHandler) allowedLibraryIDs(ctx context.Context) ([]string, error) {
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	if h.userAccess == nil {
		// nil reads as "unrestricted" below; a wiring regression must deny
		// instead, like a real zero-access user would get.
		return []string{}, nil
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
		Id:        n.ID,
		ProfileId: string(n.ProfileID),
		Type:      string(n.Type),
		Title:     n.Title,
		Subject:   n.Subject,
		// #nosec G115 -- counts of seasons/episodes in one batch, never near int32.
		SeasonCount: int32(n.SeasonCount),
		// #nosec G115 -- same bound as SeasonCount.
		EpisodeCount: int32(n.EpisodeCount),
		Message:      n.Message,
		MediaItemId:  string(n.MediaItemID),
		Level:        string(n.Level),
		CreatedAt:    toTimestamppb(n.CreatedAt),
		ExpiresAt:    toTimestamppbPtr(n.ExpiresAt),
		ReadAt:       toTimestamppbPtr(n.ReadAt),
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
