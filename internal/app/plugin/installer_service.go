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

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
)

// nativeBinaryPerm is the manifest permission a plugin must declare to
// be allowed to ship compiled binaries or embedded archives. Without it
// the installer rejects any plugin that the static analyzer flagged
// with `compiled_binary` or `embedded_archive` categories — opaque blobs
// the regex scanner cannot inspect.
const nativeBinaryPerm = "native-binary"

type InstallerService struct {
	plugins      plugin.PluginRepository
	runner       plugin.PluginRunner
	parser       plugin.ManifestParser
	events       shared.EventBus
	installer    plugin.PluginInstaller
	analyzer     plugin.PluginAnalyzer
	fs           plugin.PluginFileSystem
	cssSanitizer plugin.CSSSanitizer
	permChecker  *PermissionChecker
	pluginDir    string
	installMu    sync.Mutex
}

type InstallerOption func(*InstallerService)

func InstallerWithInstaller(installer plugin.PluginInstaller) InstallerOption {
	return func(s *InstallerService) { s.installer = installer }
}

func InstallerWithAnalyzer(analyzer plugin.PluginAnalyzer) InstallerOption {
	return func(s *InstallerService) { s.analyzer = analyzer }
}

func InstallerWithPermissionChecker(pc *PermissionChecker) InstallerOption {
	return func(s *InstallerService) { s.permChecker = pc }
}

func InstallerWithPluginDir(dir string) InstallerOption {
	return func(s *InstallerService) { s.pluginDir = dir }
}

func InstallerWithFileSystem(fs plugin.PluginFileSystem) InstallerOption {
	return func(s *InstallerService) { s.fs = fs }
}

func InstallerWithCSSSanitizer(cs plugin.CSSSanitizer) InstallerOption {
	return func(s *InstallerService) { s.cssSanitizer = cs }
}

func NewInstallerService(
	plugins plugin.PluginRepository,
	runner plugin.PluginRunner,
	parser plugin.ManifestParser,
	events shared.EventBus,
	opts ...InstallerOption,
) *InstallerService {
	s := &InstallerService{
		plugins:   plugins,
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

	if p.InstallDir != "" && slices.Contains(p.Capabilities, "ui.theme") && s.cssSanitizer != nil {
		themePath := filepath.Join(p.InstallDir, "theme.css")
		if raw, readErr := os.ReadFile(themePath); readErr == nil {
			sanitized, warns := s.cssSanitizer.SanitizeCSS(raw)
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
			// Fail-closed: without a clean analysis we cannot grant install —
			// a third-party plugin could ship obfuscated malware that the
			// regex analyzer choked on and we'd otherwise install it blind.
			slog.Error("plugin static analysis failed", slog.String("pluginID", p.ID), slog.Any("err", analyzeErr))
			if s.fs != nil {
				s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
			}
			return nil, fmt.Errorf("plugin blocked: static analysis failed: %w", analyzeErr)
		}
		if !report.Safe {
			if s.fs != nil {
				s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
			}
			return nil, fmt.Errorf("plugin blocked: critical security issues found: %v", report.Issues)
		}
		// Native binaries and archives bypass the static analyzer (they
		// are opaque blobs). Block installation by default — the manifest
		// must explicitly request the `native-binary` permission so the
		// admin sees and accepts the elevated risk at preview time.
		if !slices.Contains(p.Permissions, nativeBinaryPerm) {
			if report.HasCategory("compiled_binary") || report.HasCategory("embedded_archive") {
				if s.fs != nil {
					s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
				}
				return nil, fmt.Errorf("plugin blocked: bundles compiled binaries or archives — declare permission %q in manifest to opt in", nativeBinaryPerm)
			}
		}
		// Persist non-blocking warnings so the admin can audit them
		// post-install (compiled binaries, symlinks, base64 blobs,
		// suspicious URLs). UI exposes these on plugin detail.
		p.AnalysisReport = report
	}

	if err := s.plugins.Create(ctx, p); err != nil {
		if p.InstallDir != "" && s.fs != nil {
			s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
		}
		return nil, fmt.Errorf("InstallerService.InstallPlugin: %w", err)
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
		return fmt.Errorf("InstallerService.UninstallPlugin: %w", err)
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
