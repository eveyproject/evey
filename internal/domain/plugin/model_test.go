package plugin_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

func TestPlugin_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		p       plugin.Plugin
		wantErr bool
	}{
		{"valid", plugin.Plugin{Name: "my-plugin", Version: "1.0.0"}, false},
		{"missing name", plugin.Plugin{Version: "1.0.0"}, true},
		{"missing version", plugin.Plugin{Name: "my-plugin"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.p.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPluginManifest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		m       plugin.PluginManifest
		wantErr bool
	}{
		{"valid", plugin.PluginManifest{Name: "plg", Version: "1.0.0", Entrypoint: "main.py"}, false},
		{"missing name", plugin.PluginManifest{Version: "1.0.0", Entrypoint: "main.py"}, true},
		{"missing version", plugin.PluginManifest{Name: "plg", Entrypoint: "main.py"}, true},
		{"missing entrypoint", plugin.PluginManifest{Name: "plg", Version: "1.0.0"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
