package httpapi

import (
	"testing"

	"github.com/evey-project/evey/internal/config"
)

// Production greys nothing, the public demo greys everything (shared account),
// the extended demo is a normal server with the network settings piloted.
func TestSettingLocked_PerDemoLevel(t *testing.T) {
	mk := func(demo, ext bool) *config.Config {
		c := &config.Config{}
		c.Server.Demo, c.Server.DemoExtended = demo, ext
		return c
	}
	for _, c := range []struct {
		name      string
		cfg       *config.Config
		key       string
		wantLocks bool
	}{
		{"production", mk(false, false), "loginRateLimit", false},
		{"public demo, network", mk(true, false), "loginRateLimit", true},
		{"public demo, other", mk(true, false), "serverName", true},
		{"extended, login rate limit", mk(true, true), "loginRateLimit", true},
		{"extended, global rate limit", mk(true, true), "globalRateLimit", true},
		{"extended, server name", mk(true, true), "serverName", false},
		{"extended, transcodes", mk(true, true), "maxConcurrentTranscodes", false},
	} {
		if got := settingLocked(c.cfg, c.key); got != c.wantLocks {
			t.Errorf("%s: settingLocked(%q) = %v, want %v", c.name, c.key, got, c.wantLocks)
		}
	}
}
