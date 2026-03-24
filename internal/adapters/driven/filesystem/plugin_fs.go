package filesystem

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	pluginUID   = 1001
	sdkBasePath = "/app/sdk"
)

type PluginFileSystem struct{}

func NewPluginFileSystem() *PluginFileSystem {
	return &PluginFileSystem{}
}

func (fs *PluginFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *PluginFileSystem) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

func (fs *PluginFileSystem) CopySDKStubs(language, installDir string) error {
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

func (fs *PluginFileSystem) ChownPluginDir(dir string) {
	if err := filepath.Walk(dir, func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chown(path, pluginUID, pluginUID)
	}); err != nil {
		slog.Warn("chown plugin dir", slog.String("dir", dir), slog.Any("err", err))
	}
}
