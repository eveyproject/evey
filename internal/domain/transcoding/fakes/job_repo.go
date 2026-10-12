package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

type FakeTranscodeJobRepository struct {
	mu    sync.RWMutex
	items map[string]*transcoding.TranscodeJob
	Err   error
}

func NewFakeTranscodeJobRepository() *FakeTranscodeJobRepository {
	return &FakeTranscodeJobRepository{items: make(map[string]*transcoding.TranscodeJob)}
}

func (r *FakeTranscodeJobRepository) Create(_ context.Context, job *transcoding.TranscodeJob) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[job.ID] = job
	return nil
}

func (r *FakeTranscodeJobRepository) GetByID(_ context.Context, id string) (*transcoding.TranscodeJob, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	j, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *j
	return &copy, nil
}

func (r *FakeTranscodeJobRepository) Update(_ context.Context, job *transcoding.TranscodeJob) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[job.ID]; !ok {
		return ErrNotFound
	}
	r.items[job.ID] = job
	return nil
}

func (r *FakeTranscodeJobRepository) ListActive(_ context.Context) ([]*transcoding.TranscodeJob, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*transcoding.TranscodeJob
	for _, j := range r.items {
		if j.Status == transcoding.Running || j.Status == transcoding.Queued {
			result = append(result, j)
		}
	}
	return result, nil
}

func (r *FakeTranscodeJobRepository) ListByMediaItem(_ context.Context, mediaItemID string) ([]*transcoding.TranscodeJob, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*transcoding.TranscodeJob
	for _, j := range r.items {
		if string(j.MediaItemID) == mediaItemID {
			result = append(result, j)
		}
	}
	return result, nil
}
