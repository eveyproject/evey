package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
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
	r.items[req.Code] = req
	return nil
}

func (r *FakeQuickConnectRepository) GetByCode(_ context.Context, code string) (*user.QuickConnectRequest, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	req, ok := r.items[code]
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (r *FakeQuickConnectRepository) UpdateStatus(_ context.Context, id, status string, userID *string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.items {
		if req.ID == id {
			req.Status = status
			req.UserID = userID
			return nil
		}
	}
	return ErrNotFound
}

func (r *FakeQuickConnectRepository) DeleteExpired(_ context.Context) error {
	if r.Err != nil {
		return r.Err
	}
	return nil
}
