package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/webhook"
)

type FakeDeliveryRepository struct {
	mu    sync.RWMutex
	items map[string]*webhook.Delivery
	Err   error
}

func NewFakeDeliveryRepository() *FakeDeliveryRepository {
	return &FakeDeliveryRepository{items: make(map[string]*webhook.Delivery)}
}

func (r *FakeDeliveryRepository) Create(_ context.Context, d *webhook.Delivery) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[d.ID] = d
	return nil
}

func (r *FakeDeliveryRepository) ListByWebhook(_ context.Context, webhookID string, limit int) ([]*webhook.Delivery, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*webhook.Delivery
	for _, d := range r.items {
		if d.WebhookID == webhookID {
			result = append(result, d)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
