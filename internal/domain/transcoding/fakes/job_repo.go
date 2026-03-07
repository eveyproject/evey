package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

type FakeTranscodeJobRepository struct {
	mu    sync.RWMutex
	items map[string]*transcoding.TranscodeJob
}

func NewFakeTranscodeJobRepository() *FakeTranscodeJobRepository {
	return &FakeTranscodeJobRepository{items: make(map[string]*transcoding.TranscodeJob)}
}

func (r *FakeTranscodeJobRepository) Create(_ context.Context, job *transcoding.TranscodeJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[job.ID] = job
	return nil
}

func (r *FakeTranscodeJobRepository) GetByID(_ context.Context, id string) (*transcoding.TranscodeJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	j, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return j, nil
}

func (r *FakeTranscodeJobRepository) Update(_ context.Context, job *transcoding.TranscodeJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[job.ID]; !ok {
		return ErrNotFound
	}
	r.items[job.ID] = job
	return nil
}

func (r *FakeTranscodeJobRepository) ListActive(_ context.Context) ([]*transcoding.TranscodeJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*transcoding.TranscodeJob
	for _, j := range r.items {
		if j.Status == string(transcoding.Running) || j.Status == string(transcoding.Queued) {
			result = append(result, j)
		}
	}
	return result, nil
}

func (r *FakeTranscodeJobRepository) ListByMediaItem(_ context.Context, mediaItemID string) ([]*transcoding.TranscodeJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*transcoding.TranscodeJob
	for _, j := range r.items {
		if j.MediaItemID == mediaItemID {
			result = append(result, j)
		}
	}
	return result, nil
}
