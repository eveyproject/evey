package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
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
	fs          plugin.PluginFileSystem
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

func (s *InstallerService) WithFileSystem(fs plugin.PluginFileSystem) *InstallerService {
	s.fs = fs
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
	SourceHash   string
}

func (s *InstallerService) InstallPlugin(ctx context.Context, zipPath string) (*plugin.Plugin, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()

	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	existing, getErr := s.plugins.GetByName(ctx, manifest.Name)
	if getErr == nil && existing != nil {
		cmp := CompareSemver(manifest.Version, existing.Version)
		if cmp <= 0 {
			return nil, fmt.Errorf("plugin already installed at version %s, new version must be higher", existing.Version)
		}
		if err := s.UninstallPlugin(ctx, existing.ID); err != nil {
			return nil, fmt.Errorf("uninstall old version: %w", err)
		}
		slog.Info("upgrading plugin", slog.String("name", manifest.Name),
			slog.String("from", existing.Version), slog.String("to", manifest.Version))
	}

	sourceHash, hashErr := hashFile(zipPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash plugin file: %w", hashErr)
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
		SourceHash:   sourceHash,
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

	if p.InstallDir != "" && slices.Contains(p.Capabilities, "ui.theme") {
		themePath := filepath.Join(p.InstallDir, "theme.css")
		if raw, readErr := os.ReadFile(themePath); readErr == nil {
			sanitized, warns := pluginrunner.SanitizeCSS(raw)
			if len(warns) > 0 {
				slog.Warn("css sanitizer cleaned theme at install", slog.String("pluginID", p.ID), slog.Any("warnings", warns))
				if writeErr := os.WriteFile(themePath, sanitized, 0o644); writeErr != nil {
					slog.Warn("write sanitized theme.css", slog.String("pluginID", p.ID), slog.Any("err", writeErr))
				}
			}
		}
	}

	if p.InstallDir != "" && s.fs != nil {
		if cpErr := s.fs.CopySDKStubs(p.Language, p.InstallDir); cpErr != nil {
			slog.Warn("copy SDK stubs", slog.String("pluginID", p.ID), slog.Any("err", cpErr))
		}
	}

	if p.InstallDir != "" && os.Getuid() == 0 && s.fs != nil {
		s.fs.ChownPluginDir(p.InstallDir)
	}

	if s.analyzer != nil && p.InstallDir != "" {
		report, analyzeErr := s.analyzer.Analyze(p.InstallDir)
		if analyzeErr != nil {
			slog.Warn("plugin static analysis failed", slog.String("pluginID", p.ID), slog.Any("err", analyzeErr))
		} else if !report.Safe {
			if p.InstallDir != "" && s.fs != nil {
				s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
			}
			return nil, fmt.Errorf("plugin blocked: critical security issues found: %v", report.Issues)
		}
	}

	if err := s.plugins.Create(ctx, p); err != nil {
		if p.InstallDir != "" && s.fs != nil {
			s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
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
	sourceHash, hashErr := hashFile(zipPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash plugin file: %w", hashErr)
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
		SourceHash:   sourceHash,
	}
	if s.installer != nil && s.analyzer != nil && s.fs != nil {
		tmpDir, tmpErr := s.fs.MkdirTemp("", "evey-plugin-preview-*")
		if tmpErr == nil {
			defer func() {
				if rmErr := s.fs.RemoveAll(tmpDir); rmErr != nil {
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

	if p.InstallDir != "" && s.fs != nil {
		if err := s.fs.RemoveAll(p.InstallDir); err != nil {
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

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CompareSemver compares two semver strings (X.Y.Z). Returns -1, 0, or 1.
// Pre-release labels are ignored for numeric comparison.
func CompareSemver(a, b string) int {
	pa := parseSemverParts(a)
	pb := parseSemverParts(b)
	for i := range 3 {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseSemverParts(v string) [3]int {
	// strip pre-release label
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := range 3 {
		if i < len(parts) {
			n, _ := strconv.Atoi(parts[i])
			result[i] = n
		}
	}
	return result
}
