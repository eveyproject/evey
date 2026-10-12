package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	// A freshly-installed non-theme plugin is already spawned by the installer
	// (status → Initializing) and is connecting to the host socket. Starting it
	// again here would spawn a SECOND process; the first one then fails the
	// SO_PEERCRED PID auth ("PID mismatch - possible spoofing attempt") because
	// SetExpectedPID was re-armed with the second PID. Treat Initializing as
	// already-started so an enable-after-install is a no-op, not a double-spawn.
	if p.Status == plugin.Running || p.Status == plugin.Initializing {
		return ErrAlreadyRunning
	}
	if p.IsTheme() {
		p.Status = plugin.Running
		p.ErrorMessage = ""
		metrics.SetPluginUp(p.ID, true)
		return s.plugins.Update(ctx, p)
	}
	if p.InstallDir == "" {
		p.InstallDir = filepath.Join(s.pluginDir, p.ID)
	}
	if err := s.runner.Start(ctx, p); err != nil {
		p.Status = plugin.Error
		p.ErrorMessage = err.Error()
		metrics.IncPluginError(p.ID)
		metrics.SetPluginUp(p.ID, false)
		if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
			slog.WarnContext(ctx, "update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
		}
		return fmt.Errorf("RuntimeService.StartPlugin: %w", err)
	}
	if s.permChecker != nil {
		s.permChecker.LoadPermissions(p.ID, p.Permissions)
	}
	p.Status = plugin.Running
	p.ErrorMessage = ""
	metrics.SetPluginUp(p.ID, true)
	if err := s.plugins.Update(ctx, p); err != nil {
		slog.WarnContext(ctx, "update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}
	evtStarted := plugin.PluginStarted{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStarted); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
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
	// IsStarted, not Running only: a plugin still in Initializing has a live
	// process, so an admin must be able to stop it (and a stuck-in-Initializing
	// plugin must be stoppable at all).
	if !p.Status.IsStarted() {
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
		slog.WarnContext(ctx, "update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}
	evtStopped := plugin.PluginStopped{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStopped); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtStopped.EventName()), slog.Any("err", err))
	}
	return nil
}

// RestartRunningPlugins bounces every currently-running (non-theme) plugin so a
// changed start-time setting - chiefly the sandbox mode - takes effect now
// instead of lazily on the next manual restart. Best-effort and fire-and-forget:
// a plugin that fails to start under the new mode is left in Error status
// (visible in the dashboard), so the admin can revert the setting.
func (s *RuntimeService) RestartRunningPlugins(ctx context.Context) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.RestartRunningPlugins")
	defer span.End()
	plugins, err := s.plugins.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "restart running plugins: list failed", slog.Any("err", err))
		return
	}
	for _, p := range plugins {
		if p.Status != plugin.Running || p.IsTheme() {
			continue
		}
		if err := s.StopPlugin(ctx, p.ID); err != nil {
			slog.WarnContext(ctx, "restart running plugins: stop failed", slog.String("pluginID", p.ID), slog.Any("err", err))
			continue
		}
		if err := s.StartPlugin(ctx, p.ID); err != nil {
			// StartPlugin already set the plugin to Error + persisted it, so the
			// failure surfaces in the dashboard; just record it and move on.
			slog.WarnContext(ctx, "restart running plugins: start failed under new mode", slog.String("pluginID", p.ID), slog.Any("err", err))
		}
	}
}

// EnablePlugin marks the plugin enabled and (for non-theme plugins) starts
// the runner. Without the runner.Start call the plugin remained Stopped after
// re-enabling - matching DB state but not actually serving traffic.
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
	// Themes are "running" without a runner - flip the status so the UI
	// reflects that the theme is active.
	if p.IsTheme() {
		p.Status = plugin.Running
		p.ErrorMessage = ""
		metrics.SetPluginUp(p.ID, true)
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
	// IsStarted, not Running only: disabling a plugin still in Initializing must
	// stop its live process too, otherwise it kept running after being disabled.
	if p.Status.IsStarted() {
		if err := s.runner.Stop(ctx, id); err != nil {
			slog.WarnContext(ctx, "stop plugin runner failed", slog.String("pluginID", id), slog.Any("err", err))
		}
		p.Status = plugin.Stopped
	}
	p.Enabled = false
	p.UpdatedAt = time.Now()
	return s.plugins.Update(ctx, p)
}

// AutoStartPlugins starts all plugins that were running before shutdown, plus
// those left in Error.
//
// Le retry des Error est délibéré : un boot est un état neuf, et la cause d'un
// échec de lancement est souvent transitoire (contention I/O du scan initial,
// runtime pas encore prêt). Sans ça, un unique ratage parquait le plugin en
// Error DÉFINITIVEMENT - weather-v2 est resté mort 3 jours en prod (07-22 → 07-25)
// et aucun des 3 reboots ne l'a récupéré. Un plugin réellement cassé se remettra
// en Error avec un message frais, ce qui vaut mieux qu'un message vieux de 3 jours.
// Un plugin DÉSACTIVÉ reste à l'arrêt : c'est une décision de l'admin.
func (s *RuntimeService) AutoStartPlugins(ctx context.Context) {
	all, err := s.plugins.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list plugins for auto-start", slog.Any("err", err))
		return
	}
	for _, p := range all {
		if !p.Enabled || (p.Status != plugin.Running && p.Status != plugin.Initializing && p.Status != plugin.Error) {
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
				slog.WarnContext(ctx, "persist rebuilt plugin fields", slog.String("pluginID", p.ID), slog.Any("err", upErr))
			}
		}
		retryingAfterError := p.Status == plugin.Error
		slog.InfoContext(ctx, "auto-starting plugin", slog.String("pluginID", p.ID), slog.String("name", p.Name),
			slog.Bool("retry_after_error", retryingAfterError))
		if startErr := s.runner.Start(ctx, p); startErr != nil {
			slog.WarnContext(ctx, "auto-start plugin failed", slog.String("pluginID", p.ID), slog.Any("err", startErr))
			p.Status = plugin.Error
			p.ErrorMessage = startErr.Error()
			metrics.IncPluginError(p.ID)
			metrics.SetPluginUp(p.ID, false)
			if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
				slog.WarnContext(ctx, "update plugin status", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
			}
			continue
		}
		if s.permChecker != nil {
			s.permChecker.LoadPermissions(p.ID, p.Permissions)
		}
		// Un plugin repris depuis Error : le process est lancé mais l'IPC n'est
		// pas encore établi. Passer en Initializing et effacer le message du
		// boot précédent, sinon le panel admin continue d'afficher la panne
		// périmée comme si elle était en cours jusqu'à ce que HandleIPCResult
		// tranche. Les plugins déjà Running/Initializing ne sont PAS touchés :
		// les repasser en Initializing élargirait à chaque boot la fenêtre où
		// DisablePlugin (qui n'arrête le runner que sur Running) laisserait un
		// process orphelin.
		if retryingAfterError {
			p.Status = plugin.Initializing
			p.ErrorMessage = ""
			if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
				slog.WarnContext(ctx, "update plugin status", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
			}
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
		metrics.IncPluginError(p.ID)
		metrics.SetPluginUp(p.ID, false)
	} else {
		p.Status = plugin.Running
		p.ErrorMessage = ""
		metrics.SetPluginUp(p.ID, true)
	}
	if upErr := s.plugins.Update(ctx, p); upErr != nil {
		slog.WarnContext(ctx, "update plugin status after IPC", slog.String("pluginID", pluginID), slog.Any("err", upErr))
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
