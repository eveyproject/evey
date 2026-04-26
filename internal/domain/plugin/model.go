package plugin

import (
	"errors"
	"time"
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
	Name         string
	Version      string
	Description  string
	Author       string
	Language     string
	Entrypoint   string
	InstallDir   string
	Capabilities []string
	Permissions  []string
	UIWidgets    []UIWidget
	ConfigSchema map[string]any
	Config       map[string]any
	SourceHash   string
	Enabled      bool
	Status       PluginStatus
	ErrorMessage string
	InstalledAt  time.Time
	UpdatedAt    time.Time
	// AnalysisReport captures the static-analysis output from the install step
	// so the admin can audit non-critical warnings (compiled binaries, symlinks,
	// base64 blobs, suspicious URLs) after the plugin is live. Nil on legacy rows.
	AnalysisReport *AnalysisReport
	// Signature holds the cosign verification result from install time.
	// Nil when CosignPolicy is "off" or no key is configured.
	Signature *SignatureStatus
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
		ID:           id,
		Name:         manifest.Name,
		Version:      manifest.Version,
		Description:  manifest.Description,
		Author:       manifest.Author,
		Language:     manifest.Language,
		Entrypoint:   manifest.Entrypoint,
		Capabilities: manifest.Capabilities,
		Permissions:  manifest.Permissions,
		ConfigSchema: manifest.ConfigSchema,
		UIWidgets:    manifest.UIWidgets,
		Config:       make(map[string]any),
		SourceHash:   sourceHash,
		Enabled:      true,
		Status:       Stopped,
		InstalledAt:  now,
		UpdatedAt:    now,
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
	// CSS-only themes have no executable entrypoint.
	if m.Language != "css" && m.Entrypoint == "" {
		return errors.New("manifest requires an entrypoint")
	}
	return nil
}

type UIWidget struct {
	Slot string `yaml:"slot" json:"slot"`
	Src  string `yaml:"src" json:"src"`
}

type PluginManifest struct {
	Name           string         `yaml:"name"`
	Version        string         `yaml:"version"`
	Description    string         `yaml:"description"`
	Author         string         `yaml:"author"`
	Language       string         `yaml:"language"`
	Entrypoint     string         `yaml:"entrypoint"`
	Capabilities   []string       `yaml:"capabilities"`
	Permissions    []string       `yaml:"permissions"`
	MinEveyVersion string         `yaml:"min_evey_version"`
	ConfigSchema   map[string]any `yaml:"config_schema"`
	UIWidgets      []UIWidget     `yaml:"ui_widgets"`
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
	Issues      []string      // pre-formatted "[SEVERITY] file:line — description (pattern)"
	Categories  []string      // unique finding categories — used by the installer to gate native-binary opt-in
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
