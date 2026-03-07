package plugin

import "time"

type PluginStatus string

const (
	Stopped PluginStatus = "stopped"
	Running PluginStatus = "running"
	Error   PluginStatus = "error"
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
	ConfigSchema map[string]interface{}
	Config       map[string]interface{}
	Enabled      bool
	Status       string
	ErrorMessage string
	InstalledAt  time.Time
	UpdatedAt    time.Time
}

type PluginManifest struct {
	Name           string                 `yaml:"name"`
	Version        string                 `yaml:"version"`
	Description    string                 `yaml:"description"`
	Author         string                 `yaml:"author"`
	Language       string                 `yaml:"language"`
	Entrypoint     string                 `yaml:"entrypoint"`
	Capabilities   []string               `yaml:"capabilities"`
	Permissions    []string               `yaml:"permissions"`
	MinEveyVersion string                 `yaml:"min_evey_version"`
	ConfigSchema   map[string]interface{} `yaml:"config_schema"`
}

type PluginRuntime struct {
	ID       string
	Language string
	Version  string
	Status   string
	Notes    string
	TestedAt time.Time
}
