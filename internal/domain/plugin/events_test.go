package plugin_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{plugin.PluginInstalled{}, "plugin.installed"},
		{plugin.PluginUninstalled{}, "plugin.uninstalled"},
		{plugin.PluginStarted{}, "plugin.started"},
		{plugin.PluginStopped{}, "plugin.stopped"},
		{plugin.PluginError{}, "plugin.error"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
