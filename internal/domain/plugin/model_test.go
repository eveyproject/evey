package plugin_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/plugin"
)

func TestPlugin_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		p       plugin.Plugin
		wantErr bool
	}{
		{"valid", plugin.Plugin{ID: "plg-1", Name: "my-plugin", Version: "1.0.0"}, false},
		{"missing id", plugin.Plugin{Name: "my-plugin", Version: "1.0.0"}, true},
		{"missing name", plugin.Plugin{ID: "plg-1", Version: "1.0.0"}, true},
		{"missing version", plugin.Plugin{ID: "plg-1", Name: "my-plugin"}, true},
		{"invalid status", plugin.Plugin{ID: "plg-1", Name: "my-plugin", Version: "1.0.0", Status: "banana"}, true},
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

func TestNewPluginFromManifest(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		m := &plugin.PluginManifest{
			Name:        "my-plugin",
			Kind:        plugin.KindPlugin,
			Version:     "1.0.0",
			Description: "d",
			Author:      "a",
			Language:    "go",
			Entrypoint:  "main.go",
		}
		p, err := plugin.NewPluginFromManifest("plg-1", "sha256-xxx", m)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if p.ID != "plg-1" || p.SourceHash != "sha256-xxx" {
			t.Errorf("identity fields wrong: %+v", p)
		}
		if p.Status != plugin.Stopped || !p.Enabled {
			t.Errorf("defaults wrong: status=%q enabled=%v", p.Status, p.Enabled)
		}
		if p.Config == nil {
			t.Error("Config map should be initialised")
		}
		if p.InstalledAt.IsZero() || p.UpdatedAt.IsZero() {
			t.Error("timestamps not stamped")
		}
	})

	t.Run("nil manifest", func(t *testing.T) {
		t.Parallel()
		_, err := plugin.NewPluginFromManifest("plg-1", "x", nil)
		if err == nil {
			t.Fatal("expected error on nil manifest")
		}
	})

	t.Run("invalid manifest", func(t *testing.T) {
		t.Parallel()
		_, err := plugin.NewPluginFromManifest("plg-1", "x", &plugin.PluginManifest{Name: ""})
		if err == nil {
			t.Fatal("expected error on invalid manifest")
		}
	})
}

func TestPluginStatus_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[plugin.PluginStatus]bool{
		plugin.Stopped:             true,
		plugin.Initializing:        true,
		plugin.Running:             true,
		plugin.Error:               true,
		plugin.Installed:           true,
		plugin.PluginStatus(""):    false,
		"garbage":                  false,
	}
	for s, want := range cases {
		if s.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", s, !want, want)
		}
	}
}

func TestPluginManifest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		m       plugin.PluginManifest
		wantErr bool
	}{
		{"valid plugin", plugin.PluginManifest{Name: "plg", Kind: plugin.KindPlugin, Version: "1.0.0", Entrypoint: "main.py"}, false},
		{"valid theme", plugin.PluginManifest{Name: "thm", Kind: plugin.KindTheme, Version: "1.0.0"}, false},
		{"missing name", plugin.PluginManifest{Kind: plugin.KindPlugin, Version: "1.0.0", Entrypoint: "main.py"}, true},
		{"missing version", plugin.PluginManifest{Name: "plg", Kind: plugin.KindPlugin, Entrypoint: "main.py"}, true},
		{"missing kind", plugin.PluginManifest{Name: "plg", Version: "1.0.0", Entrypoint: "main.py"}, true},
		{"missing entrypoint", plugin.PluginManifest{Name: "plg", Kind: plugin.KindPlugin, Version: "1.0.0"}, true},
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
