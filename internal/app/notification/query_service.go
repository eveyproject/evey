package notification

import (
	"context"

	"github.com/evey-project/evey/internal/domain/notification"
)

type QueryService struct {
	repo     notification.Repository
	settings notification.SettingsRepository
	subs     notification.PushSubscriptionRepository
}

type QueryOption func(*QueryService)

func WithPushSubs(subs notification.PushSubscriptionRepository) QueryOption {
	return func(s *QueryService) { s.subs = subs }
}

func NewQueryService(repo notification.Repository, settings notification.SettingsRepository, opts ...QueryOption) *QueryService {
	s := &QueryService{repo: repo, settings: settings}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// List returns notifications for the personal feed only — admin-only types
// (plugin_error, backup_failed) are filtered out so they don't pollute the
// bell dropdown / profile page. Admins should consume those via the
// dedicated admin notifications channel.
//
// Implementation: over-fetches the repo by 4× the requested limit then
// filters down. Sufficient for typical UI lists (5-50 items); if a profile
// ends up with hundreds of admin-only events ahead of personal ones, the
// caller should paginate. Capped at 1000 over-fetch to bound memory.
func (s *QueryService) List(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
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
		if n != nil && n.Type.IsAdminOnly() {
			continue
		}
		out = append(out, n)
		if len(out) >= want {
			break
		}
	}
	return out, nil
}

// ListAdminOnly returns admin-only notifications (plugin_error, backup_failed)
// owned by the given admin profile, the inverse of List. Same over-fetch
// strategy: callers pass a sane limit; we fetch up to 4× and filter to the
// admin-only subset, capped at 1000 to bound memory.
//
// Caller is responsible for RBAC — this service does no role check; the
// calling handler must gate by RoleAdmin.
func (s *QueryService) ListAdminOnly(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
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

// UnreadCount returns the count of unread *personal* notifications. We
// over-fetch and count in memory rather than adding a Repository method
// because the rate (~once per page load) and the typical unread count
// (single-digit to low-hundreds) make the cost negligible.
func (s *QueryService) UnreadCount(ctx context.Context, profileID string) (int, error) {
	raw, err := s.repo.ListByProfile(ctx, profileID, true, 1000, 0)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, n := range raw {
		if n != nil && n.Type.IsAdminOnly() {
			continue
		}
		count++
	}
	return count, nil
}

func (s *QueryService) GetSettings(ctx context.Context, profileID string) (*notification.Settings, error) {
	return s.settings.GetByProfile(ctx, profileID)
}

func (s *QueryService) GetGlobalSettings(ctx context.Context) (*notification.Settings, error) {
	return s.settings.GetGlobalDefaults(ctx)
}

func (s *QueryService) ListPushSubscriptions(ctx context.Context, userID string) ([]notification.PushSubscription, error) {
	if s.subs == nil {
		return []notification.PushSubscription{}, nil
	}
	return s.subs.ListByUser(ctx, userID)
}
