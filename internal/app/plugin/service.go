package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

const defaultPluginBaseDir = "/data/plugins"

var (
	ErrPluginNotFound = errors.New("plugin not found")
	ErrAlreadyRunning = errors.New("plugin already running")
	ErrNotRunning     = errors.New("plugin not running")
)

type Service struct {
	plugins    plugin.PluginRepository
	runtimes   plugin.PluginRuntimeRepository
	runner     plugin.PluginRunner
	parser     plugin.ManifestParser
	events     shared.EventBus
	installer  plugin.PluginInstaller
	analyzer    plugin.PluginAnalyzer
	permChecker *PermissionChecker
	pluginDir   string
}

func NewService(
	plugins plugin.PluginRepository,
	runtimes plugin.PluginRuntimeRepository,
	runner plugin.PluginRunner,
	parser plugin.ManifestParser,
	events shared.EventBus,
) *Service {
	return &Service{
		plugins:   plugins,
		runtimes:  runtimes,
		runner:    runner,
		parser:    parser,
		events:    events,
		pluginDir: defaultPluginBaseDir,
	}
}

func (s *Service) WithInstaller(installer plugin.PluginInstaller) *Service {
	s.installer = installer
	return s
}

func (s *Service) WithAnalyzer(analyzer plugin.PluginAnalyzer) *Service {
	s.analyzer = analyzer
	return s
}

func (s *Service) WithPermissionChecker(pc *PermissionChecker) *Service {
	s.permChecker = pc
	return s
}

func (s *Service) WithPluginDir(dir string) *Service {
	s.pluginDir = dir
	return s
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
		Entrypoint:   manifest.Entrypoint,
		Capabilities: manifest.Capabilities,
		Permissions:  manifest.Permissions,
		ConfigSchema: manifest.ConfigSchema,
		Config:       make(map[string]any),
		Enabled:      true,
		Status:       string(plugin.Stopped),
		InstalledAt:  time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Extract ZIP to install dir
	if s.installer != nil {
		installDir := filepath.Join(s.pluginDir, p.ID)
		if err := s.installer.Extract(zipPath, installDir); err != nil {
			return nil, fmt.Errorf("extract plugin: %w", err)
		}
		p.InstallDir = installDir
	}

	// Copy pre-generated SDK stubs into plugin dir
	if p.InstallDir != "" {
		if cpErr := copySDKStubs(p.Language, p.InstallDir); cpErr != nil {
			slog.Warn("copy SDK stubs", slog.String("pluginID", p.ID), slog.Any("err", cpErr))
		}
	}

	// Run static analysis — block on critical issues
	if s.analyzer != nil && p.InstallDir != "" {
		report, analyzeErr := s.analyzer.Analyze(p.InstallDir)
		if analyzeErr != nil {
			slog.Warn("plugin static analysis failed", slog.String("pluginID", p.ID), slog.Any("err", analyzeErr))
		} else if !report.Safe {
			if p.InstallDir != "" {
				os.RemoveAll(p.InstallDir)
			}
			return nil, fmt.Errorf("plugin blocked: critical security issues found: %v", report.Issues)
		}
	}

	if err := s.plugins.Create(ctx, p); err != nil {
		return nil, err
	}

	// CSS-only plugins (themes) don't need a subprocess
	if !isStaticPlugin(p) {
		if err := s.runner.Start(ctx, p); err != nil {
			p.Status = string(plugin.Error)
			p.ErrorMessage = err.Error()
			if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
				slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
			}
			return p, nil
		}
		if s.permChecker != nil {
			s.permChecker.LoadPermissions(p.ID, p.Permissions)
		}
		p.Status = string(plugin.Running)
	} else {
		p.Status = string(plugin.Running)
	}
	if err := s.plugins.Update(ctx, p); err != nil {
		slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}

	evtInstalled := plugin.PluginInstalled{PluginID: p.ID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtInstalled); err != nil {
		slog.Warn("publish event", slog.String("event", evtInstalled.EventName()), slog.Any("err", err))
	}
	return p, nil
}

// isStaticPlugin returns true for plugins that don't need a subprocess (CSS themes).
func isStaticPlugin(p *plugin.Plugin) bool {
	return strings.EqualFold(p.Language, "css")
}

const sdkBasePath = "/app/sdk"

// copySDKStubs copies pre-generated gRPC stubs from sdk/{language}/ into the plugin install dir.
// Stubs are generated once via `make plugin-sdk` and versioned in the repo.
func copySDKStubs(language, installDir string) error {
	lang := strings.ToLower(language)
	switch lang {
	case "nodejs", "node":
		lang = "nodejs"
	case "css":
		return nil // themes don't need stubs
	}

	sdkDir := filepath.Join(sdkBasePath, lang)
	if _, statErr := os.Stat(sdkDir); statErr != nil {
		slog.Debug("no SDK stubs for language", slog.String("language", lang), slog.Any("err", statErr))
		return nil
	}

	entries, err := os.ReadDir(sdkDir)
	if err != nil {
		return fmt.Errorf("read sdk dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "README.md" || e.Name() == "VERSION" {
			continue
		}
		src := filepath.Join(sdkDir, e.Name())
		data, readErr := os.ReadFile(src)
		if readErr != nil {
			return fmt.Errorf("read sdk file %s: %w", e.Name(), readErr)
		}
		dst := filepath.Join(installDir, e.Name())
		if writeErr := os.WriteFile(dst, data, 0644); writeErr != nil {
			return fmt.Errorf("write %s: %w", e.Name(), writeErr)
		}
	}

	slog.Info("copied SDK stubs", slog.String("language", lang), slog.String("dir", installDir))
	return nil
}

func (s *Service) UninstallPlugin(ctx context.Context, id string) error {
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}
	if p.Status == string(plugin.Running) {
		if err := s.runner.Stop(ctx, id); err != nil {
			slog.Warn("stop plugin runner failed", slog.String("pluginID", id), slog.Any("err", err))
		}
	}

	if s.permChecker != nil {
		s.permChecker.InvalidatePermissions(p.ID)
	}

	// Remove install directory
	if p.InstallDir != "" {
		if err := os.RemoveAll(p.InstallDir); err != nil {
			slog.Warn("remove plugin dir", slog.String("pluginID", id), slog.Any("err", err))
		}
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
		if err := s.runner.Stop(ctx, id); err != nil {
			slog.Warn("stop plugin runner failed", slog.String("pluginID", id), slog.Any("err", err))
		}
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

func (s *Service) ListRuntimes(ctx context.Context) ([]*plugin.PluginRuntime, error) {
	return s.runtimes.List(ctx)
}

func (s *Service) AddRuntime(ctx context.Context, rt *plugin.PluginRuntime) error {
	return s.runtimes.Create(ctx, rt)
}

func (s *Service) RemoveRuntime(ctx context.Context, id string) error {
	return s.runtimes.Delete(ctx, id)
}

// AutoStartPlugins starts all plugins that were running before shutdown.
func (s *Service) AutoStartPlugins(ctx context.Context) {
	all, err := s.plugins.List(ctx)
	if err != nil {
		slog.Error("list plugins for auto-start", slog.Any("err", err))
		return
	}
	for _, p := range all {
		if !p.Enabled || p.Status != string(plugin.Running) {
			continue
		}
		if isStaticPlugin(p) {
			continue
		}
		// Rebuild missing fields (plugins installed before migration 42)
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
