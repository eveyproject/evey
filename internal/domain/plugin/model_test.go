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
		plugin.Stopped:          true,
		plugin.Initializing:     true,
		plugin.Running:          true,
		plugin.Error:            true,
		plugin.Installed:        true,
		plugin.PluginStatus(""): false,
		"garbage":               false,
	}
	for s, want := range cases {
		if s.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", s, !want, want)
		}
	}
}

func TestPlugin_PublicConfig(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"city":       map[string]any{"type": "string"},
		"units":      map[string]any{"type": "select"},
		"enabled":    map[string]any{"type": "boolean"},
		"api_key":    map[string]any{"type": "password"},               // secret by type
		"webhook":    map[string]any{"type": "string", "secret": true}, // secret by flag
		"api_key_ci": map[string]any{"type": "PASSWORD"},               // type match is case-insensitive
	}

	t.Run("withholds secrets, keeps declared non-secret keys", func(t *testing.T) {
		t.Parallel()
		p := plugin.Plugin{
			ConfigSchema: schema,
			Config: map[string]any{
				"city":       "Paris",
				"units":      "metric",
				"enabled":    true,
				"api_key":    "SECRET",
				"webhook":    "https://internal/hook",
				"api_key_ci": "SECRET2",
			},
		}
		got := p.PublicConfig()
		want := map[string]any{"city": "Paris", "units": "metric", "enabled": true}
		if len(got) != len(want) {
			t.Fatalf("PublicConfig size = %d (%v), want %d (%v)", len(got), got, len(want), want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("PublicConfig[%q] = %v, want %v", k, got[k], v)
			}
		}
		for _, secret := range []string{"api_key", "webhook", "api_key_ci"} {
			if _, ok := got[secret]; ok {
				t.Errorf("secret key %q leaked into PublicConfig", secret)
			}
		}
	})

	t.Run("withholds keys absent from the schema", func(t *testing.T) {
		t.Parallel()
		p := plugin.Plugin{
			ConfigSchema: map[string]any{"city": map[string]any{"type": "string"}},
			Config:       map[string]any{"city": "Paris", "ad_hoc_secret": "leak-me"},
		}
		got := p.PublicConfig()
		if _, ok := got["ad_hoc_secret"]; ok {
			t.Error("ad-hoc key not in schema leaked into PublicConfig")
		}
		if got["city"] != "Paris" {
			t.Errorf("declared key dropped: got %v", got)
		}
	})

	t.Run("nil config yields empty, non-nil map", func(t *testing.T) {
		t.Parallel()
		p := plugin.Plugin{ConfigSchema: schema}
		got := p.PublicConfig()
		if got == nil {
			t.Fatal("PublicConfig returned nil map")
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})
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

func TestPluginManifest_Validate_SlotWhitelist(t *testing.T) {
	t.Parallel()

	t.Run("allowed slot accepted", func(t *testing.T) {
		t.Parallel()
		m := plugin.PluginManifest{
			Name:       "slot-ok",
			Kind:       plugin.KindPlugin,
			Version:    "1.0.0",
			Entrypoint: "main.py",
			WebRoot:    "ui",
			Frontend: []plugin.FrontModule{
				{Slot: "home.section", Src: "ui/front.js"},
			},
		}
		if err := m.Validate(); err != nil {
			t.Errorf("expected no error for whitelisted slot, got: %v", err)
		}
	})

	t.Run("critical slot rejected", func(t *testing.T) {
		t.Parallel()
		m := plugin.PluginManifest{
			Name:       "slot-bad",
			Kind:       plugin.KindPlugin,
			Version:    "1.0.0",
			Entrypoint: "main.py",
			WebRoot:    "ui",
			Frontend: []plugin.FrontModule{
				{Slot: "admin.users", Src: "ui/front.js"},
			},
		}
		err := m.Validate()
		if err == nil {
			t.Fatal("expected error for non-whitelisted slot, got nil")
		}
		want := `manifest: frontend slot "admin.users" not allowed`
		if err.Error() != want {
			t.Errorf("error message mismatch\ngot:  %q\nwant: %q", err.Error(), want)
		}
	})
}
