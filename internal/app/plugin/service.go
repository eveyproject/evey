package plugin

import (
	"context"
	"errors"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const defaultPluginBaseDir = "/data/plugins"

var (
	ErrPluginNotFound = errors.New("plugin not found")
	ErrAlreadyRunning = errors.New("plugin already running")
	ErrNotRunning     = errors.New("plugin not running")
	// ErrCandidateChanged is returned by ApplyUpdate when the candidate ZIP
	// hash no longer matches the hash the admin acknowledged in PreviewUpdate.
	ErrCandidateChanged = errors.New("candidate changed since preview, retry")
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
	ctx, span := tracing.StartSpan(ctx, "app.plugin.ListPlugins")
	defer span.End()

	return s.plugins.List(ctx)
}

type PluginConfigWithSchema struct {
	Config       map[string]any
	ConfigSchema map[string]any
}

func (s *QueryService) GetPluginConfig(ctx context.Context, id string) (*PluginConfigWithSchema, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.GetPluginConfig")
	defer span.End()

	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return nil, ErrPluginNotFound
	}
	return &PluginConfigWithSchema{
		Config:       p.Config,
		ConfigSchema: p.ConfigSchema,
	}, nil
}

func (s *QueryService) ListRuntimes(ctx context.Context) ([]*plugin.PluginRuntime, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.ListRuntimes")
	defer span.End()

	return s.runtimes.List(ctx)
}
