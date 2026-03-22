package plugin

import (
	"context"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

const defaultPluginBaseDir = "/data/plugins"

var (
	ErrPluginNotFound = errors.New("plugin not found")
	ErrAlreadyRunning = errors.New("plugin already running")
	ErrNotRunning     = errors.New("plugin not running")
)

// QueryService handles read-only plugin operations.
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

func (s *QueryService) ListRuntimes(ctx context.Context) ([]*plugin.PluginRuntime, error) {
	return s.runtimes.List(ctx)
}
