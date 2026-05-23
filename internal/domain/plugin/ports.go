package plugin

import (
	"context"
	"time"
)

type PluginRepository interface {
	Create(ctx context.Context, p *Plugin) error
	GetByID(ctx context.Context, id string) (*Plugin, error)
	GetByName(ctx context.Context, name string) (*Plugin, error)
	List(ctx context.Context) ([]*Plugin, error)
	Update(ctx context.Context, p *Plugin) error
	Delete(ctx context.Context, id string) error
}

// PluginDataEntry is one row of a plugin's own key/value store. UserID is the
// empty string for plugin-global data and a user id for per-user data (e.g.
// plugin-namespaced preferences).
type PluginDataEntry struct {
	PluginID  string
	UserID    string
	Key       string
	Value     string
	UpdatedAt time.Time
}

// PluginDataRepository is a plugin's own persistence: a key/value store
// namespaced per plugin (and optionally per user). Rows cascade-delete with the
// plugin (on uninstall) and with the user, so a plugin never leaks data past
// its own lifecycle. userID == "" addresses the plugin-global scope.
type PluginDataRepository interface {
	Get(ctx context.Context, pluginID, userID, key string) (*PluginDataEntry, error)
	Set(ctx context.Context, pluginID, userID, key, value string) error
	Delete(ctx context.Context, pluginID, userID, key string) error
	List(ctx context.Context, pluginID, userID string) ([]*PluginDataEntry, error)
}

type PluginRuntimeRepository interface {
	Create(ctx context.Context, rt *PluginRuntime) error
	GetByLanguageAndVersion(ctx context.Context, language, version string) (*PluginRuntime, error)
	ListByLanguage(ctx context.Context, language string) ([]*PluginRuntime, error)
	List(ctx context.Context) ([]*PluginRuntime, error)
	Delete(ctx context.Context, id string) error
}

type PluginRunner interface {
	Start(ctx context.Context, p *Plugin) error
	Stop(ctx context.Context, pluginID string) error
	// Kill sends SIGKILL to the entire process group of the plugin, bypassing
	// the graceful gRPC shutdown path. Used as a fallback when Stop times out
	// or returns an error. Returns nil if the plugin is not tracked (already
	// dead or never started).
	Kill(pluginID string) error
	Health(ctx context.Context, pluginID string) (bool, error)
	SendEvent(ctx context.Context, pluginID string, event any) error
}

type ManifestParser interface {
	Parse(zipPath string) (*PluginManifest, error)
	ParseDir(dir string) (*PluginManifest, error)
}

type PluginInstaller interface {
	Extract(zipPath, destDir string) error
}

type PluginLogger interface {
	WriteLog(pluginID, line string)
}

type PluginAnalyzer interface {
	// Analyze walks the plugin directory and emits findings. webRoot, when
	// non-empty, marks a subdir whose contents are bundled frontend assets
	// served as-is to the browser — those files are scanned for size/URL
	// extraction but skip the source-code pattern rules (regex matching on
	// minified bundles produces noise that drowns the signal). The backend
	// entrypoint and any unbundled scripts outside webRoot stay fully
	// scrutinised.
	Analyze(dir, webRoot string) (*AnalysisReport, error)
}

type FrontCompiler interface {
	CompileFront(srcPath, outPath string) error
}

// CSSSanitizer removes dangerous CSS constructs from plugin themes.
type CSSSanitizer interface {
	SanitizeCSS(input []byte) ([]byte, []string)
}

// PluginFileSystem abstracts filesystem operations for plugin install/uninstall.
type PluginFileSystem interface {
	RemoveAll(path string) error
	MkdirTemp(dir, pattern string) (string, error)
	CopySDKStubs(language, installDir string) error
	ChownPluginDir(dir string)
}

// SignatureVerifier checks the cosign signature of a plugin ZIP.
// zipPath is the local path to the ZIP file.
// sigPath is the path to the detached signature file (empty = zipPath+".sig").
// Returns (nil, nil) when verification is disabled (policy="off" or no key configured).
type SignatureVerifier interface {
	VerifySignature(ctx context.Context, zipPath, sigPath string) (*SignatureStatus, error)
}

// RuntimeAvailabilityChecker reports which plugin runtime languages the
// host image actually supports. Used at install time to fail fast with an
// actionable message instead of letting the plugin crash at exec.
//
// IsAvailable returns true for languages with a usable interpreter on PATH
// (or compiled-only languages like "go" that need no runtime). Available
// returns the sorted list of supported languages so the installer can
// surface them in the error message ("install evey:beta-full to get php").
type RuntimeAvailabilityChecker interface {
	IsAvailable(language string) bool
	Available() []string
}
