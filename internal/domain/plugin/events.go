package plugin

import "time"

type PluginInstalled struct {
	PluginID   string
	Name       string
	OccurredAt time.Time
}

func (e PluginInstalled) EventName() string { return "plugin.installed" }

type PluginUninstalled struct {
	PluginID   string
	OccurredAt time.Time
}

func (e PluginUninstalled) EventName() string { return "plugin.uninstalled" }

type PluginStarted struct {
	PluginID   string
	OccurredAt time.Time
}

func (e PluginStarted) EventName() string { return "plugin.started" }

type PluginStopped struct {
	PluginID   string
	OccurredAt time.Time
}

func (e PluginStopped) EventName() string { return "plugin.stopped" }

type PluginError struct {
	PluginID     string
	ErrorMessage string
	OccurredAt   time.Time
}

func (e PluginError) EventName() string { return "plugin.error" }
