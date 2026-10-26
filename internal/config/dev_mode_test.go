package config

import "testing"

// The dev relaxations (sandbox optional, hardening errors downgraded, public
// rate limits lifted) need the environment to say dev too: the flag alone,
// copied from a dev compose file onto a real server, must not turn them on.
func TestDevModeNeedsDevEnv(t *testing.T) {
	for _, c := range []struct {
		flag, env string
		want      bool
	}{
		{"", "", false},
		{"true", "", false},
		{"true", "production", false},
		{"true", "staging", false},
		{"false", "dev", false},
		{"true", "dev", true},
		{"true", " DEV ", true},
	} {
		t.Setenv("EVEY_LOG_DEV_MODE", c.flag)
		t.Setenv("EVEY_ENV", c.env)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Log.DevMode != c.want {
			t.Errorf("EVEY_LOG_DEV_MODE=%q EVEY_ENV=%q: DevMode=%v, want %v", c.flag, c.env, cfg.Log.DevMode, c.want)
		}
	}
}
