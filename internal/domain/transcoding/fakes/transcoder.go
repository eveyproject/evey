package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

var ErrNotFound = shared.ErrNotFound

type FakeTranscoder struct {
	mu       sync.RWMutex
	jobs     map[string]*transcoding.TranscodeJob
	progress map[string]*transcoding.TranscodeProgress
	nextID   int
	Err      error
	// PauseCalls / ResumeCalls record jobIDs in call order for assertions.
	PauseCalls  []string
	ResumeCalls []string
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
	job.Status = transcoding.Running
	t.jobs[id] = job
	return id, nil
}

func (t *FakeTranscoder) Stop(_ context.Context, jobID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.jobs[jobID]; !ok {
		return ErrNotFound
	}
	return nil
}

func (t *FakeTranscoder) Pause(_ context.Context, jobID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.jobs[jobID]; !ok {
		return ErrNotFound
	}
	t.PauseCalls = append(t.PauseCalls, jobID)
	return nil
}

func (t *FakeTranscoder) Resume(_ context.Context, jobID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.jobs[jobID]; !ok {
		return ErrNotFound
	}
	t.ResumeCalls = append(t.ResumeCalls, jobID)
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
