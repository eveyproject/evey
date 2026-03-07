package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/playback"
)

type FakePlaySessionRepository struct {
	mu    sync.RWMutex
	items map[string]*playback.PlaySession
}

func NewFakePlaySessionRepository() *FakePlaySessionRepository {
	return &FakePlaySessionRepository{items: make(map[string]*playback.PlaySession)}
}

func (r *FakePlaySessionRepository) Create(_ context.Context, session *playback.PlaySession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[session.ID] = session
	return nil
}

func (r *FakePlaySessionRepository) Update(_ context.Context, session *playback.PlaySession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[session.ID]; !ok {
		return ErrNotFound
	}
	r.items[session.ID] = session
	return nil
}

func (r *FakePlaySessionRepository) GetByID(_ context.Context, id string) (*playback.PlaySession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

func (r *FakePlaySessionRepository) GetActiveByProfile(_ context.Context, profileID string) ([]*playback.PlaySession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playback.PlaySession
	for _, s := range r.items {
		if s.ProfileID == profileID && s.EndedAt == nil {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *FakePlaySessionRepository) GetHistory(_ context.Context, filter playback.PlaySessionFilter) ([]*playback.PlaySession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playback.PlaySession
	for _, s := range r.items {
		if filter.ProfileID != nil && s.ProfileID != *filter.ProfileID {
			continue
		}
		if filter.StartFrom != nil && s.StartedAt.Before(*filter.StartFrom) {
			continue
		}
		if filter.StartTo != nil && s.StartedAt.After(*filter.StartTo) {
			continue
		}
		result = append(result, s)
	}
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return nil, nil
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *FakePlaySessionRepository) CountActiveByUser(_ context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, s := range r.items {
		if s.EndedAt == nil {
			count++
		}
	}
	return count, nil
}
