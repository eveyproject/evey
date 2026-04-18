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
	Status       string
	ErrorMessage string
	InstalledAt  time.Time
	UpdatedAt    time.Time
	// AnalysisReport captures the static-analysis output from the install step
	// so the admin can audit non-critical warnings (compiled binaries, symlinks,
	// base64 blobs, suspicious URLs) after the plugin is live. Nil on legacy rows.
	AnalysisReport *AnalysisReport
}

func (p *Plugin) Validate() error {
	if p.Name == "" {
		return errors.New("plugin requires a name")
	}
	if p.Version == "" {
		return errors.New("plugin requires a version")
	}
	return nil
}

func (m *PluginManifest) Validate() error {
	if m.Name == "" {
		return errors.New("manifest requires a name")
	}
	if m.Version == "" {
		return errors.New("manifest requires a version")
	}
	if m.Entrypoint == "" {
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

type AnalysisReport struct {
	Safe      bool
	Issues    []string
	URLs      []ExtractedURL
	FileCount int
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
	Status   string
	Notes    string
	TestedAt time.Time
}
