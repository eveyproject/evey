package pluginrunner

import (
	"strings"
	"testing"
)

// The V8 heap ceiling must track the resource monitor's RSS cap so node
// garbage-collects below the kill threshold instead of being killed for
// crossing it.
func TestBuildPluginEnv_CapsNodeHeapBelowKillThreshold(t *testing.T) {
	env := buildPluginEnv("plg-x", "/tmp/s.sock", "/tmp/h.sock", 256<<20)
	var nodeOpts string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "NODE_OPTIONS="); ok {
			nodeOpts = v
		}
	}
	if nodeOpts != "--max-old-space-size=192" {
		t.Errorf("256 MiB cap should yield --max-old-space-size=192, got %q", nodeOpts)
	}
}

func TestBuildPluginEnv_NoCapNoNodeOptions(t *testing.T) {
	for _, e := range buildPluginEnv("plg-x", "s", "h", 0) {
		if strings.HasPrefix(e, "NODE_OPTIONS=") {
			t.Errorf("no cap should not inject NODE_OPTIONS, got %q", e)
		}
	}
}
