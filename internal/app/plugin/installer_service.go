package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

type InstallerService struct {
	plugins     plugin.PluginRepository
	runner      plugin.PluginRunner
	parser      plugin.ManifestParser
	events      shared.EventBus
	installer   plugin.PluginInstaller
	analyzer    plugin.PluginAnalyzer
	permChecker *PermissionChecker
	pluginDir   string
	installMu   sync.Mutex
}

func NewInstallerService(
	plugins plugin.PluginRepository,
	runner plugin.PluginRunner,
	parser plugin.ManifestParser,
	events shared.EventBus,
) *InstallerService {
	return &InstallerService{
		plugins:   plugins,
		runner:    runner,
		parser:    parser,
		events:    events,
		pluginDir: defaultPluginBaseDir,
	}
}

func (s *InstallerService) WithInstaller(installer plugin.PluginInstaller) *InstallerService {
	s.installer = installer
	return s
}

func (s *InstallerService) WithAnalyzer(analyzer plugin.PluginAnalyzer) *InstallerService {
	s.analyzer = analyzer
	return s
}

func (s *InstallerService) WithPermissionChecker(pc *PermissionChecker) *InstallerService {
	s.permChecker = pc
	return s
}

func (s *InstallerService) WithPluginDir(dir string) *InstallerService {
	s.pluginDir = dir
	return s
}

// PluginPreview holds manifest info + analysis without installing.
type PluginPreview struct {
	Name         string
	Version      string
	Description  string
	Author       string
	Language     string
	Entrypoint   string
	Permissions  []string
	Capabilities []string
	Analysis     *plugin.AnalysisReport
}

func (s *InstallerService) InstallPlugin(ctx context.Context, zipPath string) (*plugin.Plugin, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()

	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if existing, _ := s.plugins.GetByName(ctx, manifest.Name); existing != nil {
		return nil, fmt.Errorf("plugin %q already installed (id: %s)", manifest.Name, existing.ID)
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

	if s.installer != nil {
		installDir := filepath.Join(s.pluginDir, p.ID)
		if err := s.installer.Extract(zipPath, installDir); err != nil {
			return nil, fmt.Errorf("extract plugin: %w", err)
		}
		p.InstallDir = installDir
	}

	if p.InstallDir != "" {
		if cpErr := copySDKStubs(p.Language, p.InstallDir); cpErr != nil {
			slog.Warn("copy SDK stubs", slog.String("pluginID", p.ID), slog.Any("err", cpErr))
		}
	}

	if p.InstallDir != "" && os.Getuid() == 0 {
		chownPluginDir(p.InstallDir)
	}

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
		if p.InstallDir != "" {
			os.RemoveAll(p.InstallDir)
		}
		return nil, err
	}

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
		p.Status = string(plugin.Initializing)
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

func (s *InstallerService) PreviewPlugin(_ context.Context, zipPath string) (*PluginPreview, error) {
	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	preview := &PluginPreview{
		Name:         manifest.Name,
		Version:      manifest.Version,
		Description:  manifest.Description,
		Author:       manifest.Author,
		Language:     manifest.Language,
		Entrypoint:   manifest.Entrypoint,
		Permissions:  manifest.Permissions,
		Capabilities: manifest.Capabilities,
	}
	if s.installer != nil && s.analyzer != nil {
		tmpDir, tmpErr := os.MkdirTemp("", "evey-plugin-preview-*")
		if tmpErr == nil {
			defer func() {
				if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
					slog.Warn("remove preview tmpdir", slog.Any("err", rmErr))
				}
			}()
			if extErr := s.installer.Extract(zipPath, tmpDir); extErr == nil {
				report, analyzeErr := s.analyzer.Analyze(tmpDir)
				if analyzeErr == nil {
					preview.Analysis = report
				}
			}
		}
	}
	return preview, nil
}

func (s *InstallerService) UninstallPlugin(ctx context.Context, id string) error {
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

// isStaticPlugin returns true for plugins that don't need a subprocess (CSS themes).
func isStaticPlugin(p *plugin.Plugin) bool {
	return strings.EqualFold(p.Language, "css")
}

const pluginUID = 1001

func chownPluginDir(dir string) {
	if err := filepath.Walk(dir, func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chown(path, pluginUID, pluginUID)
	}); err != nil {
		slog.Warn("chown plugin dir", slog.String("dir", dir), slog.Any("err", err))
	}
}

const sdkBasePath = "/app/sdk"

func copySDKStubs(language, installDir string) error {
	lang := strings.ToLower(language)
	switch lang {
	case "nodejs", "node":
		lang = "nodejs"
	case "css":
		return nil
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
