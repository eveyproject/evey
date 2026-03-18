package plugin

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type RuntimeService struct {
	plugins     plugin.PluginRepository
	runner      plugin.PluginRunner
	parser      plugin.ManifestParser
	events      shared.EventBus
	permChecker *PermissionChecker
	pluginDir   string
}

func NewRuntimeService(
	plugins plugin.PluginRepository,
	runner plugin.PluginRunner,
	parser plugin.ManifestParser,
	events shared.EventBus,
) *RuntimeService {
	return &RuntimeService{
		plugins:   plugins,
		runner:    runner,
		parser:    parser,
		events:    events,
		pluginDir: defaultPluginBaseDir,
	}
}

func (s *RuntimeService) WithPermissionChecker(pc *PermissionChecker) *RuntimeService {
	s.permChecker = pc
	return s
}

func (s *RuntimeService) WithPluginDir(dir string) *RuntimeService {
	s.pluginDir = dir
	return s
}

func (s *RuntimeService) StartPlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == string(plugin.Running) {
		return ErrAlreadyRunning
	}
	if isStaticPlugin(p) {
		p.Status = string(plugin.Running)
		p.ErrorMessage = ""
		return s.plugins.Update(ctx, p)
	}
	if p.InstallDir == "" {
		p.InstallDir = filepath.Join(s.pluginDir, p.ID)
	}
	if err := s.runner.Start(ctx, p); err != nil {
		p.Status = string(plugin.Error)
		p.ErrorMessage = err.Error()
		if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
			slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
		}
		return err
	}
	if s.permChecker != nil {
		s.permChecker.LoadPermissions(p.ID, p.Permissions)
	}
	p.Status = string(plugin.Running)
	p.ErrorMessage = ""
	if err := s.plugins.Update(ctx, p); err != nil {
		slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}
	evtStarted := plugin.PluginStarted{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStarted); err != nil {
		slog.Warn("publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *RuntimeService) StopPlugin(ctx context.Context, id string) error {
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
	if s.permChecker != nil {
		s.permChecker.InvalidatePermissions(p.ID)
	}
	p.Status = string(plugin.Stopped)
	if err := s.plugins.Update(ctx, p); err != nil {
		slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}
	evtStopped := plugin.PluginStopped{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStopped); err != nil {
		slog.Warn("publish event", slog.String("event", evtStopped.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *RuntimeService) EnablePlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	p.Enabled = true
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

func (s *RuntimeService) DisablePlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == string(plugin.Running) {
		if err := s.runner.Stop(ctx, id); err != nil {
			slog.Warn("stop plugin runner failed", slog.String("pluginID", id), slog.Any("err", err))
		}
		p.Status = string(plugin.Stopped)
	}
	p.Enabled = false
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

// AutoStartPlugins starts all plugins that were running before shutdown.
func (s *RuntimeService) AutoStartPlugins(ctx context.Context) {
	all, err := s.plugins.List(ctx)
	if err != nil {
		slog.Error("list plugins for auto-start", slog.Any("err", err))
		return
	}
	for _, p := range all {
		if !p.Enabled || (p.Status != string(plugin.Running) && p.Status != string(plugin.Initializing)) {
			continue
		}
		if isStaticPlugin(p) {
			continue
		}
		rebuilt := false
		if p.InstallDir == "" {
			p.InstallDir = filepath.Join(s.pluginDir, p.ID)
			rebuilt = true
		}
		if p.Entrypoint == "" {
			if m, mErr := s.parser.ParseDir(p.InstallDir); mErr == nil && m != nil {
				p.Entrypoint = m.Entrypoint
				rebuilt = true
			}
		}
		if rebuilt {
			if upErr := s.plugins.Update(ctx, p); upErr != nil {
				slog.Warn("persist rebuilt plugin fields", slog.String("pluginID", p.ID), slog.Any("err", upErr))
			}
		}
		slog.Info("auto-starting plugin", slog.String("pluginID", p.ID), slog.String("name", p.Name))
		if startErr := s.runner.Start(ctx, p); startErr != nil {
			slog.Warn("auto-start plugin failed", slog.String("pluginID", p.ID), slog.Any("err", startErr))
			p.Status = string(plugin.Error)
			p.ErrorMessage = startErr.Error()
			if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
				slog.Warn("update plugin status", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
			}
		} else if s.permChecker != nil {
			s.permChecker.LoadPermissions(p.ID, p.Permissions)
		}
	}
}
