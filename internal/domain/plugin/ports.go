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
	// UpdateConfig writes ONLY the config column. Update is a blind full-row
	// write, so a read-modify-write through it replays the caller's stale
	// snapshot over permissions, external_fetch, enabled and status. Any path
	// where the writer is the PLUGIN itself must use this, never Update.
	UpdateConfig(ctx context.Context, id string, config map[string]any) error
	Delete(ctx context.Context, id string) error
}

// PluginDataEntry is one row of a plugin's own key/value store. A per-profile
// row is identified by ProfileID alone (a profile has exactly one owner, so
// UserID is redundant and left empty there); a per-user row has UserID and no
// ProfileID; a plugin-global row has neither.
type PluginDataEntry struct {
	PluginID  string
	UserID    string
	ProfileID string
	Key       string
	Value     string
	UpdatedAt time.Time
}

// PluginDataRepository is a plugin's own persistence: a key/value store
// namespaced per plugin, and optionally per user or per profile. Rows
// cascade-delete with the plugin (on uninstall), the user, and the profile, so
// a plugin never leaks data past its own lifecycle. Scope selection, in order:
//   - profileID set   -> per-profile (userID ignored; a profile has one owner)
//   - userID set       -> per-user (shared across the user's profiles)
//   - both empty       -> plugin-global
type PluginDataRepository interface {
	Get(ctx context.Context, pluginID, userID, profileID, key string) (*PluginDataEntry, error)
	Set(ctx context.Context, pluginID, userID, profileID, key, value string) error
	Delete(ctx context.Context, pluginID, userID, profileID, key string) error
	List(ctx context.Context, pluginID, userID, profileID string) ([]*PluginDataEntry, error)
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
	// Ask sends one event the plugin answers (OnEvent's result) within
	// timeout. handled is the plugin's own word that the event was its to
	// answer; a plugin that declines returns handled false and no result.
	Ask(ctx context.Context, pluginID, eventType string, payload map[string]any, timeout time.Duration) (result map[string]any, handled bool, err error)
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
	// served as-is to the browser - those files are scanned for size/URL
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
	// Rename moves oldPath to newPath. Used to atomically swap a validated,
	// staged upgrade into the plugin's canonical dir - must be within the same
	// filesystem (both live under the plugin dir) so the rename is atomic.
	Rename(oldPath, newPath string) error
	CopySDKStubs(language, installDir string) error
	ChownPluginDir(dir string)
}

// KeyLookup resolves a key fingerprint against the trusted-publisher list.
type KeyLookup func(ctx context.Context, fingerprint string) (name string, trusted bool)

// TrustedPublisherRepository stores the signing keys an instance accepts.
type TrustedPublisherRepository interface {
	List(ctx context.Context) ([]TrustedPublisher, error)
	// Get returns shared.ErrNotFound for an unknown fingerprint.
	Get(ctx context.Context, fingerprint string) (*TrustedPublisher, error)
	// Upsert inserts or renames a key. An existing row keeps its AddedAt and
	// never loses Seeded.
	Upsert(ctx context.Context, p TrustedPublisher) error
	// Delete returns shared.ErrNotFound for an unknown fingerprint and
	// ErrSeededPublisher for a seeded one.
	Delete(ctx context.Context, fingerprint string) error
	// PruneSeeded deletes the seeded rows whose fingerprint is not in keep, so a
	// key the operator removed from the configuration stops being trusted.
	PruneSeeded(ctx context.Context, keep []string) error
}

// SignatureVerifier checks the cosign signature of a plugin ZIP against the
// key the ZIP carries (cosign.pub at its root).
// zipPath is the local path to the ZIP file.
// sigPath is the path to the detached signature file (empty = zipPath+".sig").
// Returns (nil, nil) when verification is disabled (policy="off").
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
// surface them in the error message ("the default image ships php, the -lean variant does not").
type RuntimeAvailabilityChecker interface {
	IsAvailable(language string) bool
	Available() []string
}
