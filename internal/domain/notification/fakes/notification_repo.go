package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

type FakeNotificationRepository struct {
	mu    sync.Mutex
	items []*notification.Notification
}

func NewFakeNotificationRepository() *FakeNotificationRepository {
	return &FakeNotificationRepository{}
}

func (r *FakeNotificationRepository) Create(_ context.Context, n *notification.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, n)
	return nil
}

func (r *FakeNotificationRepository) ListByProfile(_ context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (r *FakeNotificationRepository) MarkRead(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, n := range r.items {
		if n.ID == id {
			n.ReadAt = &now
			return nil
		}
	}
	return nil
}

func (r *FakeNotificationRepository) MarkAllRead(_ context.Context, profileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, n := range r.items {
		if n.ProfileID == profileID && n.ReadAt == nil {
			n.ReadAt = &now
		}
	}
	return nil
}

func (r *FakeNotificationRepository) UnreadCount(_ context.Context, profileID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, n := range r.items {
		if n.ProfileID == profileID && n.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (r *FakeNotificationRepository) DeleteAll(_ context.Context, profileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.Lock()
	defer r.mu.Unlock()
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
