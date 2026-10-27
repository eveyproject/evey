package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

type FakePluginRunner struct {
	mu      sync.RWMutex
	running map[string]bool
	killed  map[string]bool
	Err     error
	StopErr error
	KillErr error
	// AskFunc answers Ask; nil declines every event.
	AskFunc func(ctx context.Context, pluginID, eventType string, payload map[string]any, timeout time.Duration) (map[string]any, bool, error)
}

func NewFakePluginRunner() *FakePluginRunner {
	return &FakePluginRunner{running: make(map[string]bool), killed: make(map[string]bool)}
}

func (r *FakePluginRunner) Start(_ context.Context, p *plugin.Plugin) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running[p.ID] = true
	return nil
}

func (r *FakePluginRunner) Stop(_ context.Context, pluginID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.StopErr == nil {
		delete(r.running, pluginID)
	}
	return r.StopErr
}

func (r *FakePluginRunner) Kill(pluginID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.KillErr != nil {
		return r.KillErr
	}
	delete(r.running, pluginID)
	r.killed[pluginID] = true
	return nil
}

// WasKilled reports whether Kill was called for the given plugin ID.
func (r *FakePluginRunner) WasKilled(pluginID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.killed[pluginID]
}

func (r *FakePluginRunner) Health(_ context.Context, pluginID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running[pluginID], nil
}

func (r *FakePluginRunner) SendEvent(_ context.Context, _ string, _ any) error {
	return nil
}

// Ask answers with AskFunc when set, else declines.
func (r *FakePluginRunner) Ask(ctx context.Context, pluginID, eventType string, payload map[string]any, timeout time.Duration) (map[string]any, bool, error) {
	if r.AskFunc != nil {
		return r.AskFunc(ctx, pluginID, eventType, payload, timeout)
	}
	return nil, false, nil
}
