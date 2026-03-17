package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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
	analyzer   plugin.PluginAnalyzer
	pluginDir  string
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

	// Generate gRPC stubs for interpreted languages
	if p.InstallDir != "" && !isStaticPlugin(p) {
		if genErr := generateGRPCStubs(p.Language, p.InstallDir); genErr != nil {
			slog.Warn("generate gRPC stubs", slog.String("pluginID", p.ID), slog.Any("err", genErr))
		}
	}

	// Run static analysis
	if s.analyzer != nil && p.InstallDir != "" {
		report, analyzeErr := s.analyzer.Analyze(p.InstallDir)
		if analyzeErr != nil {
			slog.Warn("plugin static analysis failed", slog.String("pluginID", p.ID), slog.Any("err", analyzeErr))
		} else if !report.Safe {
			slog.Warn("plugin has security issues", slog.String("pluginID", p.ID), slog.Any("issues", report.Issues))
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

const protoSourcePath = "/app/proto/plugin/v1/plugin_api.proto"
const protoIncludePath = "/app/proto"

// generateGRPCStubs generates language-specific gRPC client/server code
// from the plugin proto, so plugin authors don't need to do it themselves.
func generateGRPCStubs(language, installDir string) error {
	lang := strings.ToLower(language)
	switch lang {
	case "python":
		return generatePythonStubs(installDir)
	case "nodejs", "node":
		// Node.js uses @grpc/proto-loader at runtime — just copy the proto
		return copyProtoFile(installDir)
	case "php":
		return generatePHPStubs(installDir)
	case "ruby":
		return generateRubyStubs(installDir)
	default:
		// Go/Rust/C plugins are compiled — stubs are built-in
		// Unknown languages get the proto file as fallback
		return copyProtoFile(installDir)
	}
}

func generatePythonStubs(installDir string) error {
	// Generate into a temp dir first, then flatten to installDir
	tmpDir := filepath.Join(installDir, ".grpc_gen_tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("python3", "-m", "grpc_tools.protoc",
		"--proto_path="+protoIncludePath,
		"--python_out="+tmpDir,
		"--grpc_python_out="+tmpDir,
		protoSourcePath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("protoc python: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	// Flatten: move generated files from plugin/v1/ to installDir root
	genDir := filepath.Join(tmpDir, "plugin", "v1")
	entries, err := os.ReadDir(genDir)
	if err != nil {
		return fmt.Errorf("read generated dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(genDir, e.Name())
		data, readErr := os.ReadFile(src)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", e.Name(), readErr)
		}
		// Fix import paths: "from plugin.v1 import" → direct import
		content := strings.ReplaceAll(string(data), "from plugin.v1 import ", "import ")
		content = strings.ReplaceAll(content, "from plugin.v1.", "from .")
		dst := filepath.Join(installDir, e.Name())
		if writeErr := os.WriteFile(dst, []byte(content), 0644); writeErr != nil {
			return fmt.Errorf("write %s: %w", e.Name(), writeErr)
		}
	}

	slog.Info("generated Python gRPC stubs", slog.String("dir", installDir))
	return nil
}

func copyProtoFile(installDir string) error {
	src, err := os.ReadFile(protoSourcePath)
	if err != nil {
		return fmt.Errorf("read proto: %w", err)
	}
	dst := filepath.Join(installDir, "plugin_api.proto")
	if err := os.WriteFile(dst, src, 0644); err != nil {
		return fmt.Errorf("write proto: %w", err)
	}
	return nil
}

func generatePHPStubs(installDir string) error {
	outDir := filepath.Join(installDir, "generated")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create php output dir: %w", err)
	}
	cmd := exec.Command("protoc",
		"--proto_path="+protoIncludePath,
		"--php_out="+outDir,
		"--grpc_out="+outDir,
		"--plugin=protoc-gen-grpc="+findGRPCPlugin("grpc_php_plugin"),
		protoSourcePath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Fallback: just copy the proto file
		slog.Debug("php protoc failed, copying proto as fallback", slog.Any("err", err))
		return copyProtoFile(installDir)
	}
	slog.Info("generated PHP gRPC stubs", slog.String("dir", installDir))
	return nil
}

func generateRubyStubs(installDir string) error {
	cmd := exec.Command("grpc_tools_ruby_protoc",
		"--proto_path="+protoIncludePath,
		"--ruby_out="+installDir,
		"--grpc_out="+installDir,
		protoSourcePath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Fallback: just copy the proto file
		slog.Debug("ruby protoc failed, copying proto as fallback", slog.Any("err", err))
		return copyProtoFile(installDir)
	}
	slog.Info("generated Ruby gRPC stubs", slog.String("dir", installDir))
	return nil
}

func findGRPCPlugin(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
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
	if err := s.runner.Start(ctx, p); err != nil {
		p.Status = string(plugin.Error)
		p.ErrorMessage = err.Error()
		if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
			slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
		}
		return err
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
		if p.Status == string(plugin.Running) && p.Enabled {
			slog.Info("auto-starting plugin", slog.String("pluginID", p.ID), slog.String("name", p.Name))
			if err := s.runner.Start(ctx, p); err != nil {
				slog.Warn("auto-start plugin failed", slog.String("pluginID", p.ID), slog.Any("err", err))
				p.Status = string(plugin.Error)
				p.ErrorMessage = err.Error()
				if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
					slog.Warn("update plugin status", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
				}
			}
		}
	}
}
