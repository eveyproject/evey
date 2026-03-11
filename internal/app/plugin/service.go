package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

var (
	ErrPluginNotFound = errors.New("plugin not found")
	ErrAlreadyRunning = errors.New("plugin already running")
	ErrNotRunning     = errors.New("plugin not running")
)

type Service struct {
	plugins  plugin.PluginRepository
	runtimes plugin.PluginRuntimeRepository
	runner   plugin.PluginRunner
	parser   plugin.ManifestParser
	events   shared.EventBus
}

func NewService(
	plugins plugin.PluginRepository,
	runtimes plugin.PluginRuntimeRepository,
	runner plugin.PluginRunner,
	parser plugin.ManifestParser,
	events shared.EventBus,
) *Service {
	return &Service{
		plugins:  plugins,
		runtimes: runtimes,
		runner:   runner,
		parser:   parser,
		events:   events,
	}
}

func (s *Service) InstallPlugin(ctx context.Context, zipPath string) (*plugin.Plugin, error) {
	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	p := &plugin.Plugin{
		ID:           id.New("plg-"),
		Name:         manifest.Name,
		Version:      manifest.Version,
		Description:  manifest.Description,
		Author:       manifest.Author,
		Language:     manifest.Language,
		Capabilities: manifest.Capabilities,
		Permissions:  manifest.Permissions,
		ConfigSchema: manifest.ConfigSchema,
		Config:       make(map[string]any),
		Enabled:      true,
		Status:       string(plugin.Stopped),
		InstalledAt:  time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.plugins.Create(ctx, p); err != nil {
		return nil, err
	}

	if err := s.runner.Start(ctx, p); err != nil {
		p.Status = string(plugin.Error)
		p.ErrorMessage = err.Error()
		s.plugins.Update(ctx, p)
		return p, nil //nolint:nilerr
	}
	p.Status = string(plugin.Running)
	s.plugins.Update(ctx, p)

	evtInstalled := plugin.PluginInstalled{PluginID: p.ID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtInstalled); err != nil {
		slog.Warn("publish event", slog.String("event", evtInstalled.EventName()), slog.Any("err", err))
	}
	return p, nil
}

func (s *Service) UninstallPlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == string(plugin.Running) {
		s.runner.Stop(ctx, id)
	}
	if err := s.plugins.Delete(ctx, id); err != nil {
		return err
	}
	evtUninstalled := plugin.PluginUninstalled{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtUninstalled); err != nil {
		slog.Warn("publish event", slog.String("event", evtUninstalled.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *Service) EnablePlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	p.Enabled = true
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

func (s *Service) DisablePlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == string(plugin.Running) {
		s.runner.Stop(ctx, id)
		p.Status = string(plugin.Stopped)
	}
	p.Enabled = false
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

func (s *Service) ListPlugins(ctx context.Context) ([]*plugin.Plugin, error) {
	return s.plugins.List(ctx)
}

func (s *Service) GetPluginConfig(ctx context.Context, id string) (map[string]any, error) {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return nil, ErrPluginNotFound
	}
	return p.Config, nil
}

func (s *Service) UpdatePluginConfig(ctx context.Context, id string, config map[string]any) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	p.Config = config
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

func (s *Service) StartPlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == string(plugin.Running) {
		return ErrAlreadyRunning
	}
	if err := s.runner.Start(ctx, p); err != nil {
		p.Status = string(plugin.Error)
		p.ErrorMessage = err.Error()
		s.plugins.Update(ctx, p)
		return err
	}
	p.Status = string(plugin.Running)
	p.ErrorMessage = ""
	s.plugins.Update(ctx, p)
	evtStarted := plugin.PluginStarted{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStarted); err != nil {
		slog.Warn("publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *Service) StopPlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status != string(plugin.Running) {
		return ErrNotRunning
	}
	if err := s.runner.Stop(ctx, id); err != nil {
		return err
	}
	p.Status = string(plugin.Stopped)
	s.plugins.Update(ctx, p)
	evtStopped := plugin.PluginStopped{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStopped); err != nil {
		slog.Warn("publish event", slog.String("event", evtStopped.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *Service) ListRuntimes(ctx context.Context) ([]*plugin.PluginRuntime, error) {
	return s.runtimes.List(ctx)
}

func (s *Service) AddRuntime(ctx context.Context, rt *plugin.PluginRuntime) error {
	return s.runtimes.Create(ctx, rt)
}

func (s *Service) RemoveRuntime(ctx context.Context, id string) error {
	return s.runtimes.Delete(ctx, id)
}
