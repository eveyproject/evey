package config

import "testing"

// A bench instance runs no plugin, whatever EVEY_PLUGINS_ENABLED says: a
// plugin is background work like any other.
func TestBenchInstanceTurnsPluginsOff(t *testing.T) {
	for _, c := range []struct {
		bench, plugins string
		wantBench      bool
		wantPlugins    bool
	}{
		{"", "", false, true},
		{"", "true", false, true},
		{"true", "", true, false},
		{"1", "true", true, false},
		{"false", "true", false, true},
	} {
		t.Setenv("EVEY_BENCH_INSTANCE", c.bench)
		t.Setenv("EVEY_PLUGINS_ENABLED", c.plugins)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Server.BenchInstance != c.wantBench || cfg.Server.PluginsEnabled != c.wantPlugins {
			t.Errorf("EVEY_BENCH_INSTANCE=%q EVEY_PLUGINS_ENABLED=%q: bench=%v plugins=%v, want %v %v",
				c.bench, c.plugins, cfg.Server.BenchInstance, cfg.Server.PluginsEnabled, c.wantBench, c.wantPlugins)
		}
	}
}
