package fakes

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeNotificationRepository struct {
	mu    sync.Mutex
	items []*notification.Notification
	Err   error
}

func NewFakeNotificationRepository() *FakeNotificationRepository {
	return &FakeNotificationRepository{}
}

func (r *FakeNotificationRepository) Create(_ context.Context, n *notification.Notification) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items = append(r.items, n)
	return nil
}

func (r *FakeNotificationRepository) ListByProfile(_ context.Context, profileID shared.ProfileID, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var out []*notification.Notification
	for _, n := range r.items {
		if n.ProfileID != profileID {
			continue
		}
		if unreadOnly && n.ReadAt != nil {
			continue
		}
		out = append(out, n)
	}
	if offset >= len(out) {
		return nil, nil
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *FakeNotificationRepository) MarkRead(_ context.Context, id string, profileID shared.ProfileID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	now := time.Now()
	for _, n := range r.items {
		if n.ID == id && n.ProfileID == profileID {
			n.ReadAt = &now
			return nil
		}
	}
	return nil
}

func (r *FakeNotificationRepository) MarkAllRead(_ context.Context, profileID shared.ProfileID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	now := time.Now()
	for _, n := range r.items {
		if n.ProfileID == profileID && n.ReadAt == nil {
			n.ReadAt = &now
		}
	}
	return nil
}

func (r *FakeNotificationRepository) UnreadCount(_ context.Context, profileID shared.ProfileID) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return 0, r.Err
	}
	count := 0
	for _, n := range r.items {
		if n.ProfileID == profileID && n.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (r *FakeNotificationRepository) UnreadCountExcludingTypes(_ context.Context, profileID shared.ProfileID, excludeTypes []notification.Type) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	excluded := make(map[notification.Type]bool, len(excludeTypes))
	for _, t := range excludeTypes {
		excluded[t] = true
	}
	count := 0
	for _, n := range r.items {
		if n.ProfileID == profileID && n.ReadAt == nil && !excluded[n.Type] {
			count++
		}
	}
	return count, nil
}

func (r *FakeNotificationRepository) DeleteAll(_ context.Context, profileID shared.ProfileID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	var out []*notification.Notification
	for _, n := range r.items {
		if n.ProfileID != profileID {
			out = append(out, n)
		}
	}
	r.items = out
	return nil
}

func (r *FakeNotificationRepository) CleanupExpired(_ context.Context) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	now := time.Now()
	var out []*notification.Notification
	for _, n := range r.items {
		if n.ExpiresAt == nil || n.ExpiresAt.After(now) {
			out = append(out, n)
		}
	}
	r.items = out
	return nil
}

func (r *FakeNotificationRepository) FindRecentByProfilesTypeAndSeries(_ context.Context, profileIDs []shared.ProfileID, types []notification.Type, relatedMediaID string, since time.Time) (map[shared.ProfileID][]*notification.Notification, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	profileSet := make(map[shared.ProfileID]bool, len(profileIDs))
	for _, id := range profileIDs {
		profileSet[id] = true
	}
	typeSet := make(map[notification.Type]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	out := make(map[shared.ProfileID][]*notification.Notification)
	for _, n := range r.items {
		if !profileSet[n.ProfileID] {
			continue
		}
		if !typeSet[n.Type] {
			continue
		}
		if string(n.RelatedMediaID) != relatedMediaID {
			continue
		}
		if n.CreatedAt.Before(since) {
			continue
		}
		out[n.ProfileID] = append(out[n.ProfileID], n)
	}
	// Mirror the SQL contract (ORDER BY created_at DESC, id DESC) so a caller
	// that reads out[p][0] as "the most recent" is validated against the same
	// ordering the real repo guarantees, not the fake's insertion order.
	for _, list := range out {
		sort.SliceStable(list, func(i, j int) bool {
			if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
				return list[i].CreatedAt.After(list[j].CreatedAt)
			}
			return list[i].ID > list[j].ID
		})
	}
	return out, nil
}

func (r *FakeNotificationRepository) Update(_ context.Context, updated *notification.Notification) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range r.items {
		if n.ID == updated.ID && n.ProfileID == updated.ProfileID {
			n.Title = updated.Title
			// Subject and SeasonCount travel with Title and EpisodeCount: the
			// postgres Update writes all four, and a fake that dropped two of
			// them would let a test pass on a regrouped notification whose
			// counts never moved.
			n.Subject = updated.Subject
			n.SeasonCount = updated.SeasonCount
			n.EpisodeCount = updated.EpisodeCount
			return nil
		}
	}
	return nil
}

// ensure compile-time interface check
var _ notification.Repository = (*FakeNotificationRepository)(nil)

// GetItems returns a copy of all stored notifications (for test assertions).
func (r *FakeNotificationRepository) GetItems() []*notification.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*notification.Notification, len(r.items))
	copy(out, r.items)
	return out
}
