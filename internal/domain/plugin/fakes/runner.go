package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/plugin"
)

type FakePluginRunner struct {
	mu      sync.RWMutex
	running map[string]bool
	Err     error
	StopErr error
}

func NewFakePluginRunner() *FakePluginRunner {
	return &FakePluginRunner{running: make(map[string]bool)}
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
	delete(r.running, pluginID)
	return r.StopErr
}

func (r *FakePluginRunner) Health(_ context.Context, pluginID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running[pluginID], nil
}

func (r *FakePluginRunner) SendEvent(_ context.Context, _ string, _ any) error {
	return nil
}
