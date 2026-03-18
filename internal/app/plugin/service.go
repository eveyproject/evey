package plugin

import (
	"context"
	"errors"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

const defaultPluginBaseDir = "/data/plugins"

var (
	ErrPluginNotFound = errors.New("plugin not found")
	ErrAlreadyRunning = errors.New("plugin already running")
	ErrNotRunning     = errors.New("plugin not running")
)

// QueryService handles read-only plugin operations and runtime config.
type QueryService struct {
	plugins  plugin.PluginRepository
	runtimes plugin.PluginRuntimeRepository
}

func NewQueryService(
	plugins plugin.PluginRepository,
	runtimes plugin.PluginRuntimeRepository,
) *QueryService {
	return &QueryService{
		plugins:  plugins,
		runtimes: runtimes,
	}
}

func (s *QueryService) ListPlugins(ctx context.Context) ([]*plugin.Plugin, error) {
	return s.plugins.List(ctx)
}

func (s *QueryService) GetPluginConfig(ctx context.Context, id string) (map[string]any, error) {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return nil, ErrPluginNotFound
	}
	return p.Config, nil
}

func (s *QueryService) UpdatePluginConfig(ctx context.Context, id string, config map[string]any) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	p.Config = config
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

func (s *QueryService) ListRuntimes(ctx context.Context) ([]*plugin.PluginRuntime, error) {
	return s.runtimes.List(ctx)
}

func (s *QueryService) AddRuntime(ctx context.Context, rt *plugin.PluginRuntime) error {
	return s.runtimes.Create(ctx, rt)
}

func (s *QueryService) RemoveRuntime(ctx context.Context, id string) error {
	return s.runtimes.Delete(ctx, id)
}
