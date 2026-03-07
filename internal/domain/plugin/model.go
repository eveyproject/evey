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
	Name           string
	Version        string
	Description    string
	Author         string
	Language       string
	Entrypoint     string
	Capabilities   []string
	Permissions    []string
	MinEveyVersion string
	ConfigSchema   map[string]interface{}
}

type PluginRuntime struct {
	ID       string
	Language string
	Version  string
	Status   string
	Notes    string
	TestedAt time.Time
}
