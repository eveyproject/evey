package fakes

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

type FakeQuickConnectRepository struct {
	mu    sync.RWMutex
	items map[string]*user.QuickConnectRequest
	Err   error
}

func NewFakeQuickConnectRepository() *FakeQuickConnectRepository {
	return &FakeQuickConnectRepository{items: make(map[string]*user.QuickConnectRequest)}
}

func (r *FakeQuickConnectRepository) Create(_ context.Context, req *user.QuickConnectRequest) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[req.Code] = req
	return nil
}

func (r *FakeQuickConnectRepository) GetByCode(_ context.Context, code string) (*user.QuickConnectRequest, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	req, ok := r.items[code]
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (r *FakeQuickConnectRepository) GetByShortCode(_ context.Context, shortCode string) (*user.QuickConnectRequest, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	upper := strings.ToUpper(shortCode)
	for _, req := range r.items {
		if req.Status == user.Pending && strings.HasSuffix(strings.ToUpper(req.Code), upper) {
			return req, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeQuickConnectRepository) UpdateStatus(_ context.Context, id, status string, userID *string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	for _, req := range r.items {
		if req.ID == id {
			req.Status = user.QuickConnectStatus(status)
			if userID != nil {
				uid := shared.UserID(*userID)
				req.UserID = &uid
			} else {
				req.UserID = nil
			}
			return nil
		}
	}
	return ErrNotFound
}

func (r *FakeQuickConnectRepository) DeleteExpired(_ context.Context) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for code, req := range r.items {
		if req.ExpiresAt.Before(now) {
			delete(r.items, code)
		}
	}
	return nil
}

func (r *FakeQuickConnectRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
