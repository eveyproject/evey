package filesystem

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/eveyproject/evey/internal/pkg/safefs"
)

// Vars, not consts, so tests can point them at a temp SDK and their own uid.
var (
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

func (fs *PluginFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (fs *PluginFileSystem) CopySDKStubs(language, installDir string) error {
	lang := strings.ToLower(language)
	switch lang {
	case "nodejs", "node":
		lang = "nodejs"
	case "css":
		return nil
	}

	if !isSafePluginLanguage(lang) {
		slog.Warn("rejected unsafe plugin language, skipping SDK stubs", slog.String("language", language))
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
		// The plugin can plant a link at the stub name (or be the link): write
		// a temp file then rename over it, never through it.
		if writeErr := safefs.WriteFileReplace(installDir, e.Name(), data, 0644); writeErr != nil {
			return fmt.Errorf("write %s: %w", e.Name(), writeErr)
		}
	}

	slog.Info("copied SDK stubs", slog.String("language", lang), slog.String("dir", installDir))
	return nil
}

// isSafePluginLanguage rejects any value that isn't a bare directory-name
// component. language comes from a plugin's manifest and feeds straight into
// filepath.Join(sdkBasePath, lang) below - "..", a path separator, or an
// absolute path would walk CopySDKStubs out of sdkBasePath entirely. This
// intentionally does not hardcode a runtime allowlist (python/nodejs/go/...):
// that set already lives in the runtime-availability check upstream, and
// duplicating it here would silently break a legitimate plugin language this
// function has never needed to know about.
func isSafePluginLanguage(lang string) bool {
	if lang == "" || strings.ContainsAny(lang, `/\`) {
		return false
	}
	if lang != filepath.Base(lang) {
		return false
	}
	return lang != "." && lang != ".."
}

func (fs *PluginFileSystem) ChownPluginDir(dir string) {
	if err := filepath.Walk(dir, func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Lchown: a link planted in the tree is re-owned itself, never its target.
		return os.Lchown(path, pluginUID, pluginUID)
	}); err != nil {
		slog.Warn("chown plugin dir", slog.String("dir", dir), slog.Any("err", err))
	}
}
