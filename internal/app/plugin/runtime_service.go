package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type RuntimeService struct {
	plugins     plugin.PluginRepository
	runtimes    plugin.PluginRuntimeRepository
	runner      plugin.PluginRunner
	parser      plugin.ManifestParser
	events      shared.EventBus
	permChecker *PermissionChecker
	pluginDir   string
	shutdownCtx context.Context
}

type RuntimeOption func(*RuntimeService)

func RuntimeWithPermissionChecker(pc *PermissionChecker) RuntimeOption {
	return func(s *RuntimeService) { s.permChecker = pc }
}

func RuntimeWithPluginDir(dir string) RuntimeOption {
	return func(s *RuntimeService) { s.pluginDir = dir }
}

func RuntimeWithShutdownCtx(ctx context.Context) RuntimeOption {
	return func(s *RuntimeService) { s.shutdownCtx = ctx }
}

func NewRuntimeService(
	plugins plugin.PluginRepository,
	runtimes plugin.PluginRuntimeRepository,
	runner plugin.PluginRunner,
	parser plugin.ManifestParser,
	events shared.EventBus,
	opts ...RuntimeOption,
) *RuntimeService {
	s := &RuntimeService{
		plugins:   plugins,
		runtimes:  runtimes,
		runner:    runner,
		parser:    parser,
		events:    events,
		pluginDir: defaultPluginBaseDir,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *RuntimeService) StartPlugin(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.StartPlugin")
	defer span.End()
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == plugin.Running {
		return ErrAlreadyRunning
	}
	if p.IsTheme() {
		p.Status = plugin.Running
		p.ErrorMessage = ""
		return s.plugins.Update(ctx, p)
	}
	if p.InstallDir == "" {
		p.InstallDir = filepath.Join(s.pluginDir, p.ID)
	}
	if err := s.runner.Start(ctx, p); err != nil {
		p.Status = plugin.Error
		p.ErrorMessage = err.Error()
		if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
			slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
		}
		return fmt.Errorf("RuntimeService.StartPlugin: %w", err)
	}
	if s.permChecker != nil {
		s.permChecker.LoadPermissions(p.ID, p.Permissions)
	}
	p.Status = plugin.Running
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
	ctx, span := tracing.StartSpan(ctx, "app.plugin.StopPlugin")
	defer span.End()
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status != plugin.Running {
		return ErrNotRunning
	}
	if err := s.runner.Stop(ctx, id); err != nil {
		return fmt.Errorf("RuntimeService.StopPlugin: %w", err)
	}
	if s.permChecker != nil {
		s.permChecker.InvalidatePermissions(p.ID)
	}
	p.Status = plugin.Stopped
	if err := s.plugins.Update(ctx, p); err != nil {
		slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}
	evtStopped := plugin.PluginStopped{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStopped); err != nil {
		slog.Warn("publish event", slog.String("event", evtStopped.EventName()), slog.Any("err", err))
	}
	return nil
}

// RestartRunningPlugins bounces every currently-running (non-theme) plugin so a
// changed start-time setting — chiefly the sandbox mode — takes effect now
// instead of lazily on the next manual restart. Best-effort and fire-and-forget:
// a plugin that fails to start under the new mode is left in Error status
// (visible in the dashboard), so the admin can revert the setting.
func (s *RuntimeService) RestartRunningPlugins(ctx context.Context) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.RestartRunningPlugins")
	defer span.End()
	plugins, err := s.plugins.List(ctx)
	if err != nil {
		slog.Warn("restart running plugins: list failed", slog.Any("err", err))
		return
	}
	for _, p := range plugins {
		if p.Status != plugin.Running || p.IsTheme() {
			continue
		}
		if err := s.StopPlugin(ctx, p.ID); err != nil {
			slog.Warn("restart running plugins: stop failed", slog.String("pluginID", p.ID), slog.Any("err", err))
			continue
		}
		if err := s.StartPlugin(ctx, p.ID); err != nil {
			// StartPlugin already set the plugin to Error + persisted it, so the
			// failure surfaces in the dashboard; just record it and move on.
			slog.Warn("restart running plugins: start failed under new mode", slog.String("pluginID", p.ID), slog.Any("err", err))
		}
	}
}

// EnablePlugin marks the plugin enabled and (for non-theme plugins) starts
// the runner. Without the runner.Start call the plugin remained Stopped after
// re-enabling — matching DB state but not actually serving traffic.
func (s *RuntimeService) EnablePlugin(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.EnablePlugin")
	defer span.End()
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	p.Enabled = true
	p.UpdatedAt = time.Now()
	if err := s.plugins.Update(ctx, p); err != nil {
		return err
	}
	if p.Status == plugin.Running {
		return nil
	}
	// Themes are "running" without a runner — flip the status so the UI
	// reflects that the theme is active.
	if p.IsTheme() {
		p.Status = plugin.Running
		p.ErrorMessage = ""
		return s.plugins.Update(ctx, p)
	}
	return s.StartPlugin(ctx, id)
}

func (s *RuntimeService) DisablePlugin(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.DisablePlugin")
	defer span.End()
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == plugin.Running {
		if err := s.runner.Stop(ctx, id); err != nil {
			slog.Warn("stop plugin runner failed", slog.String("pluginID", id), slog.Any("err", err))
		}
		p.Status = plugin.Stopped
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
		if !p.Enabled || (p.Status != plugin.Running && p.Status != plugin.Initializing) {
			continue
		}
		if p.IsTheme() {
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
			p.Status = plugin.Error
			p.ErrorMessage = startErr.Error()
			if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
				slog.Warn("update plugin status", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
			}
		} else if s.permChecker != nil {
			s.permChecker.LoadPermissions(p.ID, p.Permissions)
		}
	}
}

func (s *RuntimeService) UpdatePluginConfig(ctx context.Context, id string, config map[string]any) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.UpdatePluginConfig")
	defer span.End()
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	p.Config = config
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

func (s *RuntimeService) HandleIPCResult(pluginID string, ipcErr error) {
	parent := s.shutdownCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	p, getErr := s.plugins.GetByID(ctx, pluginID)
	if getErr != nil {
		return
	}
	if ipcErr != nil {
		p.Status = plugin.Error
		p.ErrorMessage = ipcErr.Error()
	} else {
		p.Status = plugin.Running
		p.ErrorMessage = ""
	}
	if upErr := s.plugins.Update(ctx, p); upErr != nil {
		slog.Warn("update plugin status after IPC", slog.String("pluginID", pluginID), slog.Any("err", upErr))
	}
}

func (s *RuntimeService) AddRuntime(ctx context.Context, rt *plugin.PluginRuntime) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.AddRuntime")
	defer span.End()
	return s.runtimes.Create(ctx, rt)
}

func (s *RuntimeService) RemoveRuntime(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.RemoveRuntime")
	defer span.End()
	return s.runtimes.Delete(ctx, id)
}
