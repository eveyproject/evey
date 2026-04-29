package plugin

import (
	"errors"
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
	ID           string
	Kind         PluginKind
	Name         string
	Version      string
	Description  string
	Author       string
	Language     string
	Entrypoint   string
	InstallDir   string
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
	Issues      []string       // pre-formatted "[SEVERITY] file:line — description (pattern)"
	Categories  []string       // unique finding categories — used by the installer to gate native-binary opt-in
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
// for that category — CRITICAL findings are already handled by Safe=false).
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
