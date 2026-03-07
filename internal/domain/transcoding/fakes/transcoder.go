package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

var ErrNotFound = errors.New("not found")

type FakeTranscoder struct {
	mu       sync.RWMutex
	jobs     map[string]*transcoding.TranscodeJob
	progress map[string]*transcoding.TranscodeProgress
	nextID   int
	Err      error
}

func NewFakeTranscoder() *FakeTranscoder {
	return &FakeTranscoder{
		jobs:     make(map[string]*transcoding.TranscodeJob),
		progress: make(map[string]*transcoding.TranscodeProgress),
	}
}

func (t *FakeTranscoder) Start(_ context.Context, job *transcoding.TranscodeJob) (string, error) {
	if t.Err != nil {
		return "", t.Err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	id := job.ID
	if id == "" {
		id = "fake-job-" + string(rune(t.nextID+'0'))
	}
	job.Status = string(transcoding.Running)
	t.jobs[id] = job
	return id, nil
}

func (t *FakeTranscoder) Stop(_ context.Context, jobID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	job, ok := t.jobs[jobID]
	if !ok {
		return ErrNotFound
	}
	job.Status = string(transcoding.Cancelled)
	return nil
}

func (t *FakeTranscoder) Progress(_ context.Context, jobID string) (*transcoding.TranscodeProgress, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	p, ok := t.progress[jobID]
	if !ok {
		return &transcoding.TranscodeProgress{JobID: jobID}, nil
	}
	return p, nil
}

func (t *FakeTranscoder) SetProgress(jobID string, p *transcoding.TranscodeProgress) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress[jobID] = p
}
