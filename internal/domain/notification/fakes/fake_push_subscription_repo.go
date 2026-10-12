package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type subEntry struct {
	userID    shared.UserID
	profileID shared.ProfileID
	endpoint  string
	p256dh    string
	auth      string
}

type FakePushSubscriptionRepository struct {
	mu      sync.Mutex
	entries []subEntry
	Err     error
}

func NewFakePushSubscriptionRepository() *FakePushSubscriptionRepository {
	return &FakePushSubscriptionRepository{}
}

func (r *FakePushSubscriptionRepository) Upsert(_ context.Context, userID shared.UserID, profileID shared.ProfileID, endpoint, p256dh, auth string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.entries {
		if e.endpoint == endpoint {
			r.entries[i] = subEntry{userID: userID, profileID: profileID, endpoint: endpoint, p256dh: p256dh, auth: auth}
			return nil
		}
	}
	r.entries = append(r.entries, subEntry{userID: userID, profileID: profileID, endpoint: endpoint, p256dh: p256dh, auth: auth})
	return nil
}

func (r *FakePushSubscriptionRepository) DeleteByEndpoint(_ context.Context, userID shared.UserID, endpoint string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.entries[:0]
	for _, e := range r.entries {
		if e.endpoint != endpoint || e.userID != userID {
			out = append(out, e)
		}
	}
	r.entries = out
	return nil
}

func (r *FakePushSubscriptionRepository) DeleteByUser(_ context.Context, userID shared.UserID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.entries[:0]
	for _, e := range r.entries {
		if e.userID != userID {
			out = append(out, e)
		}
	}
	r.entries = out
	return nil
}

func (r *FakePushSubscriptionRepository) GetByProfile(_ context.Context, profileID shared.ProfileID) ([]notification.PushSubscription, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []notification.PushSubscription
	for _, e := range r.entries {
		if e.profileID == profileID {
			out = append(out, notification.PushSubscription{
				UserID:    e.userID,
				ProfileID: e.profileID,
				Endpoint:  e.endpoint,
				P256dh:    e.p256dh,
				Auth:      e.auth,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			})
		}
	}
	return out, nil
}

func (r *FakePushSubscriptionRepository) ListByUser(_ context.Context, userID shared.UserID) ([]notification.PushSubscription, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []notification.PushSubscription
	for _, e := range r.entries {
		if e.userID == userID {
			out = append(out, notification.PushSubscription{
				UserID:    e.userID,
				ProfileID: e.profileID,
				Endpoint:  e.endpoint,
				P256dh:    e.p256dh,
				Auth:      e.auth,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			})
		}
	}
	return out, nil
}
