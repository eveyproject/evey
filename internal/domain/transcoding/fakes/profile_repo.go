package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

type FakeTranscodeProfileRepository struct {
	mu    sync.RWMutex
	items map[string]*transcoding.TranscodeProfile
	Err   error
}

func NewFakeTranscodeProfileRepository() *FakeTranscodeProfileRepository {
	return &FakeTranscodeProfileRepository{items: make(map[string]*transcoding.TranscodeProfile)}
}

func (r *FakeTranscodeProfileRepository) GetByID(_ context.Context, id string) (*transcoding.TranscodeProfile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakeTranscodeProfileRepository) List(_ context.Context) ([]*transcoding.TranscodeProfile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*transcoding.TranscodeProfile, 0, len(r.items))
	for _, p := range r.items {
		result = append(result, p)
	}
	return result, nil
}

func (r *FakeTranscodeProfileRepository) Create(_ context.Context, profile *transcoding.TranscodeProfile) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[profile.ID] = profile
	return nil
}

func (r *FakeTranscodeProfileRepository) Update(_ context.Context, profile *transcoding.TranscodeProfile) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[profile.ID]; !ok {
		return ErrNotFound
	}
	r.items[profile.ID] = profile
	return nil
}

func (r *FakeTranscodeProfileRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
