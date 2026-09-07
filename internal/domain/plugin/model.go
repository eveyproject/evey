package plugin

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type PluginKind string

const (
	KindTheme  PluginKind = "theme"
	KindPlugin PluginKind = "plugin"
)

type PluginStatus string

const (
	Stopped      PluginStatus = "stopped"
	Initializing PluginStatus = "initializing"
	Running      PluginStatus = "running"
	Error        PluginStatus = "error"
	Installed    PluginStatus = "installed"
)

type RuntimeStatus string

const (
	Tested      RuntimeStatus = "tested"
	Recommended RuntimeStatus = "recommended"
	Deprecated  RuntimeStatus = "deprecated"
)

type Plugin struct {
	ID          string
	Kind        PluginKind
	Name        string
	Version     string
	Description string
	Author      string
	Language    string
	Entrypoint  string
	InstallDir  string
	// WebRoot is the relative subdirectory inside InstallDir whose contents
	// are exposed via the public asset route. Required when Frontend is set;
	// any file outside that root cannot be requested by the iframe.
	WebRoot      string
	Capabilities []string
	Permissions  []string
	Frontend     []FrontModule
	ConfigSchema map[string]any
	Config       map[string]any
	SourceHash   string
	Enabled      bool
	Status       PluginStatus
	ErrorMessage string
	InstalledAt  time.Time
	UpdatedAt    time.Time
	// ExternalFetch is the DNS whitelist for plugins with the "network" permission.
	// Only hostnames declared here are reachable inside the bwrap sandbox.
	ExternalFetch []string
	// AnalysisReport captures the static-analysis output from the install step
	// so the admin can audit non-critical warnings (compiled binaries, symlinks,
	// base64 blobs, suspicious URLs) after the plugin is live. Nil on legacy rows.
	AnalysisReport *AnalysisReport
	// Signature holds the cosign verification result from install time.
	// Nil when CosignPolicy is "off" or no key is configured.
	Signature *SignatureStatus
}

// IsTheme returns true when the plugin is a pure front-end theme (no backend runtime).
func (p *Plugin) IsTheme() bool { return p.Kind == KindTheme }

// PublicConfig returns the subset of the plugin's saved configuration that is
// safe to hand to the sandboxed front-end iframe and to non-admin callers of
// GET /plugins/{id}/config. Only keys declared in the manifest's config_schema
// are returned, minus any field marked secret - i.e. a field of type "password"
// or one carrying an explicit `secret: true`. Keys absent from the schema
// (ad-hoc values an admin may have stored) and secret keys (API keys, internal
// URLs) are withheld, so secrets never reach the browser where any authenticated
// user could read them. Admins read the full config via the admin endpoint.
func (p *Plugin) PublicConfig() map[string]any {
	out := make(map[string]any)
	if len(p.Config) == 0 {
		return out
	}
	for key, schema := range p.ConfigSchema {
		if isSecretConfigField(schema) {
			continue
		}
		if v, ok := p.Config[key]; ok {
			out[key] = v
		}
	}
	return out
}

// isSecretConfigField reports whether a config_schema entry describes a value
// that must not be exposed to the front-end: fields of type "password" or any
// field with an explicit `secret: true` flag. Entries arrive as map[string]any
// both from the manifest (YAML) and from the repo (JSONB), so the type
// assertion is safe on the runtime path; an unrecognised shape is treated as
// non-secret only because such a key would not have been settable via the
// schema-driven admin form in the first place.
func isSecretConfigField(schema any) bool {
	m, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	if t, ok := m["type"].(string); ok && strings.EqualFold(strings.TrimSpace(t), "password") {
		return true
	}
	if s, ok := m["secret"].(bool); ok {
		return s
	}
	return false
}

func (p *Plugin) Validate() error {
	if p.ID == "" {
		return errors.New("plugin requires an ID")
	}
	if p.Name == "" {
		return errors.New("plugin requires a name")
	}
	if p.Version == "" {
		return errors.New("plugin requires a version")
	}
	if p.Status != "" && !p.Status.IsValid() {
		return errors.New("plugin: invalid status")
	}
	if p.Kind == KindTheme {
		if p.Language != "" {
			return errors.New("plugin: theme must not have a language")
		}
		if p.Entrypoint != "" {
			return errors.New("plugin: theme must not have an entrypoint")
		}
		if len(p.Permissions) > 0 {
			return errors.New("plugin: theme must not declare permissions")
		}
	}
	return nil
}

// NewPluginFromManifest builds a freshly-installed Plugin record from
// a validated manifest. The installer mints the ID + sourceHash (those
// can't be derived from the manifest alone) and stamps timestamps.
// Status defaults to Stopped; Enabled to true. Use this over &Plugin{...}
// literals so Validate() is enforced before hitting the repo.
func NewPluginFromManifest(id, sourceHash string, manifest *PluginManifest) (*Plugin, error) {
	if manifest == nil {
		return nil, errors.New("plugin manifest required")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	now := time.Now()
	p := &Plugin{
		ID:            id,
		Kind:          manifest.Kind,
		Name:          manifest.Name,
		Version:       manifest.Version,
		Description:   manifest.Description,
		Author:        manifest.Author,
		Language:      manifest.Language,
		Entrypoint:    manifest.Entrypoint,
		WebRoot:       manifest.WebRoot,
		Capabilities:  manifest.Capabilities,
		Permissions:   manifest.Permissions,
		ConfigSchema:  manifest.ConfigSchema,
		Frontend:      manifest.Frontend,
		ExternalFetch: manifest.ExternalFetch,
		Config:        make(map[string]any),
		SourceHash:    sourceHash,
		Enabled:       true,
		Status:        Stopped,
		InstalledAt:   now,
		UpdatedAt:     now,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (s PluginStatus) IsValid() bool {
	switch s {
	case Stopped, Initializing, Running, Error, Installed:
		return true
	}
	return false
}

// IsStarted reports whether a runner process is (or should be) alive for this
// status. A plugin auto-started on install is persisted as Initializing until
// its IPC handshake completes and it flips to Running - and a plugin that never
// completes the handshake (a broken or hostile one) sits at Initializing with a
// live process indefinitely. Lifecycle transitions that must terminate the
// process (stop, disable, uninstall) have to cover BOTH states: gating them on
// Running alone leaked the Initializing-state process (the DB row and install
// dir were removed while the OS process kept running until the next restart).
func (s PluginStatus) IsStarted() bool {
	return s == Running || s == Initializing
}

func (m *PluginManifest) Validate() error {
	if m.Name == "" {
		return errors.New("manifest requires a name")
	}
	if m.Version == "" {
		return errors.New("manifest requires a version")
	}
	switch m.Kind {
	case KindTheme:
		if m.Language != "" {
			return errors.New("manifest: theme must not have a language")
		}
		if m.Entrypoint != "" {
			return errors.New("manifest: theme must not have an entrypoint")
		}
		if len(m.Permissions) > 0 {
			return errors.New("manifest: theme must not declare permissions")
		}
	case KindPlugin:
		// Language check is intentionally delegated to the pluginrunner layer
		// where full manifest validation (semver, capabilities) is performed.
		if m.Entrypoint == "" {
			return errors.New("manifest requires an entrypoint")
		}
	default:
		return errors.New("manifest: kind is required (use 'plugin' or 'theme')")
	}
	if len(m.Frontend) > 0 {
		if m.WebRoot == "" {
			return errors.New("manifest: web_root is required when frontend is declared")
		}
		if err := validateWebRoot(m.WebRoot); err != nil {
			return err
		}
		for _, f := range m.Frontend {
			if !IsSlotAllowed(f.Slot) {
				return fmt.Errorf("manifest: frontend slot %q not allowed", f.Slot)
			}
			if err := validateSrcUnderWebRoot(m.WebRoot, f.Src); err != nil {
				return fmt.Errorf("manifest: frontend src %q: %w", f.Src, err)
			}
		}
	}
	return nil
}

// validateWebRoot rejects absolute paths, "..", or backslash separators.
// "." is allowed (means plugin root) but the pluginrunner layer warns.
func validateWebRoot(wr string) error {
	if strings.HasPrefix(wr, "/") {
		return errors.New("manifest: web_root must be a relative path")
	}
	if strings.Contains(wr, "\\") {
		return errors.New("manifest: web_root must not contain backslashes")
	}
	cleaned := filepath.Clean(wr)
	for _, seg := range strings.Split(cleaned, string(filepath.Separator)) {
		if seg == ".." {
			return errors.New("manifest: web_root must not contain '..' segments")
		}
	}
	return nil
}

// validateSrcUnderWebRoot ensures src has webRoot as a path prefix after Clean.
func validateSrcUnderWebRoot(webRoot, src string) error {
	cleanRoot := filepath.Clean(webRoot)
	cleanSrc := filepath.Clean(src)
	if cleanRoot == "." {
		return nil // plugin root - anything under it is allowed
	}
	prefix := cleanRoot + string(filepath.Separator)
	if cleanSrc != cleanRoot && !strings.HasPrefix(cleanSrc, prefix) {
		return fmt.Errorf("src must be under web_root %q", webRoot)
	}
	return nil
}

type FrontModule struct {
	Slot string `yaml:"slot" json:"slot"`
	Src  string `yaml:"src" json:"src"`
}

type PluginManifest struct {
	Name           string         `yaml:"name"`
	Kind           PluginKind     `yaml:"kind"`
	Version        string         `yaml:"version"`
	Description    string         `yaml:"description"`
	Author         string         `yaml:"author"`
	Language       string         `yaml:"language"`
	Entrypoint     string         `yaml:"entrypoint"`
	WebRoot        string         `yaml:"web_root"`
	Capabilities   []string       `yaml:"capabilities"`
	Permissions    []string       `yaml:"permissions"`
	MinEveyVersion string         `yaml:"min_evey_version"`
	ConfigSchema   map[string]any `yaml:"config_schema"`
	Frontend       []FrontModule  `yaml:"frontend"`
	ExternalFetch  []string       `yaml:"external_fetch"`
}

// PluginFinding is a single static-analysis finding with full detail.
// Stored in AnalysisReport.RawFindings and persisted as part of the analysis_report
// JSONB column, so full finding detail survives server restarts.
type PluginFinding struct {
	Category    string
	Severity    string // "CRITICAL", "WARNING", "INFO"
	File        string
	Line        int
	Description string
}

type AnalysisReport struct {
	Safe        bool
	Issues      []string        // pre-formatted "[SEVERITY] file:line - description (pattern)"
	Categories  []string        // unique finding categories - used by the installer to gate native-binary opt-in
	RawFindings []PluginFinding // per-finding detail: populated at install time, used for dashboard
	URLs        []ExtractedURL
	FileCount   int
}

// HasCategory returns true if the report contains at least one finding
// whose category matches the argument. Used by the installer to decide
// whether a plugin needs an explicit opt-in permission for compiled
// binaries or embedded archives.
func (r *AnalysisReport) HasCategory(cat string) bool {
	for _, c := range r.Categories {
		if c == cat {
			return true
		}
	}
	return false
}

// HasWarningCategory returns true if the report contains at least one
// WARNING-severity finding for the given category (and no CRITICAL finding
// for that category - CRITICAL findings are already handled by Safe=false).
// Used by the installer to escalate specific warning categories to blocking.
func (r *AnalysisReport) HasWarningCategory(cat string) bool {
	for _, f := range r.RawFindings {
		if f.Category == cat && f.Severity == "WARNING" {
			return true
		}
	}
	return false
}

type ExtractedURL struct {
	URL  string
	File string
	Line int
	Type string // "literal", "template", "concatenation"
}

type PluginRuntime struct {
	ID       string
	Language string
	Version  string
	Status   RuntimeStatus
	Notes    string
	TestedAt time.Time
}
