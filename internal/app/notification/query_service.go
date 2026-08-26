package notification

import (
	"context"

	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type QueryService struct {
	repo     notification.QueryRepo
	settings notification.SettingsRepository
	subs     notification.PushSubscriptionRepository
}

type QueryOption func(*QueryService)

func WithPushSubs(subs notification.PushSubscriptionRepository) QueryOption {
	return func(s *QueryService) { s.subs = subs }
}

// NewQueryService takes only the READ half of the store (ADR-0009): a query
// service that could write would be a category error the compiler now rejects.
func NewQueryService(repo notification.QueryRepo, settings notification.SettingsRepository, opts ...QueryOption) *QueryService {
	s := &QueryService{repo: repo, settings: settings}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// List returns the owner's notification feed. Admin-only types are NOT filtered
// out (see the body): their rows only ever exist on admin profiles, so the
// owner's feed is the single paging channel for them. Defaults an unset limit to
// 50 and pages straight through the repo - no over-fetch.
func (s *QueryService) List(ctx context.Context, profileID shared.ProfileID, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	ctx, span := tracing.StartSpan(ctx, "app.notification.List")
	defer span.End()
	want := limit
	if want <= 0 {
		want = 50
	}
	// Admin-only types are NOT filtered out here. Their rows are only ever
	// created on admin profiles (the event handlers target ListAdminProfiles),
	// so excluding them from the owner's feed excluded them from every surface
	// the product has: no client calls ListAdminNotifications, and in prod
	// (2026-08-26) media requests piled up for weeks without the operator ever
	// seeing a notification. The owner's feed is the paging channel; clients
	// that want less noise filter by type (the bell already hides
	// plugin_error, which has its own dashboard).
	return s.repo.ListByProfile(ctx, profileID, unreadOnly, want, offset)
}

// ListAdminOnly returns admin-only notifications (plugin_error, backup_failed)
// owned by the given admin profile, the inverse of List. Same over-fetch
// strategy: callers pass a sane limit; we fetch up to 4× and filter to the
// admin-only subset, capped at 1000 to bound memory.
//
// Caller is responsible for RBAC - this service does no role check; the
// calling handler must gate by RoleAdmin.
func (s *QueryService) ListAdminOnly(ctx context.Context, profileID shared.ProfileID, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	ctx, span := tracing.StartSpan(ctx, "app.notification.ListAdminOnly")
	defer span.End()
	want := limit
	if want <= 0 {
		want = 50
	}
	overFetch := want * 4
	if overFetch > 1000 {
		overFetch = 1000
	}
	raw, err := s.repo.ListByProfile(ctx, profileID, unreadOnly, overFetch, offset)
	if err != nil {
		return nil, err
	}
	out := make([]*notification.Notification, 0, len(raw))
	for _, n := range raw {
		if n == nil || !n.Type.IsAdminOnly() {
			continue
		}
		out = append(out, n)
		if len(out) >= want {
			break
		}
	}
	return out, nil
}

// UnreadCount returns the count of unread notifications via a single
// COUNT(*). Admin-only types are counted too - they exist only on admin
// profiles and the badge is what pages the operator (see List above).
func (s *QueryService) UnreadCount(ctx context.Context, profileID shared.ProfileID) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.notification.UnreadCount")
	defer span.End()
	return s.repo.UnreadCount(ctx, profileID)
}

func (s *QueryService) GetSettings(ctx context.Context, profileID shared.ProfileID) (*notification.Settings, error) {
	ctx, span := tracing.StartSpan(ctx, "app.notification.GetSettings")
	defer span.End()
	return s.settings.GetByProfile(ctx, profileID)
}

func (s *QueryService) GetGlobalSettings(ctx context.Context) (*notification.Settings, error) {
	ctx, span := tracing.StartSpan(ctx, "app.notification.GetGlobalSettings")
	defer span.End()
	return s.settings.GetGlobalDefaults(ctx)
}

func (s *QueryService) ListPushSubscriptions(ctx context.Context, userID shared.UserID) ([]notification.PushSubscription, error) {
	ctx, span := tracing.StartSpan(ctx, "app.notification.ListPushSubscriptions")
	defer span.End()
	if s.subs == nil {
		return []notification.PushSubscription{}, nil
	}
	return s.subs.ListByUser(ctx, userID)
}
