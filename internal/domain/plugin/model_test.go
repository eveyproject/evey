package plugin_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/plugin"
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

// The frontend `src` path-containment check, exercised through the public
// manifest validation. Both of its guards survived mutation testing, and this
// one decides which files a third-party plugin can expose through the asset
// route - the prefix comparison is what stops "/web" from also matching
// "/webroot-of-someone-else".
func TestPluginManifest_FrontendSrcMustStayUnderWebRoot(t *testing.T) {
	manifest := func(webRoot, src string) *plugin.PluginManifest {
		return &plugin.PluginManifest{
			Name: "p", Version: "1.0.0", Kind: plugin.KindTheme,
			WebRoot:  webRoot,
			Frontend: []plugin.FrontModule{{Slot: "home.section", Src: src}},
		}
	}

	t.Run("a file directly under the web root is allowed", func(t *testing.T) {
		if err := manifest("web", "web/app.js").Validate(); err != nil {
			t.Fatalf("unexpected rejection: %v", err)
		}
	})

	t.Run("the web root itself is allowed", func(t *testing.T) {
		if err := manifest("web", "web").Validate(); err != nil {
			t.Fatalf("unexpected rejection: %v", err)
		}
	})

	t.Run("a nested file is allowed", func(t *testing.T) {
		if err := manifest("web", "web/js/app.js").Validate(); err != nil {
			t.Fatalf("unexpected rejection: %v", err)
		}
	})

	// The separator in the prefix is the whole point: without it, a sibling
	// directory whose name merely STARTS with the web root would pass.
	t.Run("a sibling with a shared name prefix is refused", func(t *testing.T) {
		if err := manifest("web", "website/app.js").Validate(); err == nil {
			t.Fatal("web/ must not match website/")
		}
	})

	t.Run("a path escaping upwards is refused", func(t *testing.T) {
		if err := manifest("web", "web/../secrets.env").Validate(); err == nil {
			t.Fatal("a traversal out of the web root must be refused")
		}
	})

	t.Run("an unrelated path is refused", func(t *testing.T) {
		if err := manifest("web", "other/app.js").Validate(); err == nil {
			t.Fatal("a path outside the web root must be refused")
		}
	})

	// A web root of "." is the plugin root: everything under it is in scope,
	// so containment is vacuously satisfied.
	t.Run("a dot web root allows anything beneath it", func(t *testing.T) {
		if err := manifest(".", "anything/app.js").Validate(); err != nil {
			t.Fatalf("a '.' web root should allow any relative path: %v", err)
		}
	})
}

// Initializing holds a LIVE process (the IPC handshake is still pending), so
// every lifecycle transition that kills the process must treat it as started;
// gating on Running alone leaked those processes.
func TestPluginStatus_IsStarted(t *testing.T) {
	for _, s := range []plugin.PluginStatus{plugin.Running, plugin.Initializing} {
		if !s.IsStarted() {
			t.Errorf("%s must count as started", s)
		}
	}
	for _, s := range []plugin.PluginStatus{plugin.Stopped, plugin.Installed, plugin.Error} {
		if s.IsStarted() {
			t.Errorf("%s must not count as started", s)
		}
	}
}

func TestPlugin_IsTheme(t *testing.T) {
	if !(&plugin.Plugin{Kind: plugin.KindTheme}).IsTheme() {
		t.Error("a theme is a theme")
	}
	if (&plugin.Plugin{Kind: plugin.KindPlugin}).IsTheme() || (&plugin.Plugin{}).IsTheme() {
		t.Error("a backend plugin, or an unset kind, is not a theme")
	}
}

// The installer's opt-in gates read the report two ways: a category is
// present at all (HasCategory), or a WARNING finding exists for it
// (HasWarningCategory) - CRITICAL findings are already Safe=false.
func TestAnalysisReport_CategoryLookups(t *testing.T) {
	r := &plugin.AnalysisReport{
		Categories: []string{"binary", "archive"},
		RawFindings: []plugin.PluginFinding{
			{Category: "binary", Severity: "CRITICAL"},
			{Category: "archive", Severity: "WARNING"},
			{Category: "network", Severity: "INFO"},
		},
	}
	if !r.HasCategory("binary") || !r.HasCategory("archive") {
		t.Error("declared categories must be found")
	}
	if r.HasCategory("network") || r.HasCategory("") {
		t.Error("a category only present in raw findings, or an empty one, is not in Categories")
	}
	if !r.HasWarningCategory("archive") {
		t.Error("a WARNING finding must be found by category")
	}
	if r.HasWarningCategory("binary") {
		t.Error("a CRITICAL finding is not a warning: Safe=false already covers it")
	}
	if r.HasWarningCategory("network") || (&plugin.AnalysisReport{}).HasWarningCategory("archive") {
		t.Error("INFO findings and empty reports carry no warning")
	}
}
