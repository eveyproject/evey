package pluginrunner

import (
	"context"
	"os/exec"
	"slices"
	"testing"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression guard for the 2026-07-28 production outage: weather-v2 (nodejs)
// died at every boot with
//
//	runtime: failed to create new OS thread (have 6 already; errno=11)
//	runtime: may need to increase max user processes (ulimit -u)
//	fatal error: newosproc
//
// because the launcher argv carried -rlimit-as/-rlimit-nproc derived from the
// resource monitor's RSS/PID quota. Both flags are the wrong kernel primitive
// for a per-plugin quota, and the launcher's own flag documentation already
// said so ("risky for node", "per-UID, leaks across plugins sharing the UID"):
//
//   - RLIMIT_NPROC counts every task of the REAL UID, host-wide. Under
//     landlock the plugin keeps the server's uid (pluginDropArgs(false)
//     returns 0,0 unless uid-sep is "on"), so the cap is measured against the
//     server's own thread count. evey-server alone runs far past 64 tasks, so
//     the very next clone() in the launcher returned EAGAIN - before it ever
//     reached the plugin. Reproduced verbatim: `ulimit -u 64` on a uid with
//     123 live tasks kills any Go binary at startup.
//   - RLIMIT_AS caps VIRTUAL address space, not RSS. V8 reserves a multi-GB
//     cage regardless of heap use, so 256 MiB is fatal at boot:
//     "Fatal process out of memory: Failed to reserve virtual memory for
//     CodeRange". The Go runtime reserves large arenas too. Any AS value loose
//     enough for node would never bind before the RSS sweep does.
//
// Per-plugin memory and PID quotas stay where they work: the resource monitor
// polls /proc per process (procRSSBytes / procThreads) and kills on overrun.
// The launcher flags themselves are kept - an operator running a single
// non-node plugin on a dedicated uid can still pass them by hand.
func assertNoRlimitFlags(t *testing.T, args []string) {
	t.Helper()
	assert.Equal(t, -1, slices.Index(args, "-rlimit-as"),
		"-rlimit-as must not be injected: it caps virtual address space, which V8 reserves in GB")
	assert.Equal(t, -1, slices.Index(args, "-rlimit-nproc"),
		"-rlimit-nproc must not be injected: it is per-UID and counts the server's own threads")
}

func TestApplySandbox_Landlock_DoesNotInjectRlimits(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeLandlock).WithAllowMissingSandbox(true)
	r.WithResourceLimits(123*1024*1024, 42)
	p := &plugin.Plugin{ID: "weather", InstallDir: "/data/plugins/weather"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")

	cleanup, _, err := r.applySandbox(context.Background(), p, cmd)
	require.NoError(t, err)
	if cleanup != nil {
		cleanup()
	}

	assertNoRlimitFlags(t, cmd.Args)

	// The launcher wrapping itself must stay intact - only the rlimit flags go.
	sep := slices.Index(cmd.Args, "--")
	require.GreaterOrEqual(t, sep, 0, "launcher argv must still carry the -- separator")
	assert.Equal(t, "/usr/bin/node", cmd.Args[len(cmd.Args)-2])
	assert.Equal(t, "/data/plugins/weather/main.js", cmd.Args[len(cmd.Args)-1])
}

// TestApplySandbox_BasicWithLauncher_DoesNotInjectRlimits covers the other
// launcher-backed path (sandbox_mode=basic, launcher available). It shares the
// uid with the server just as often, so it carries the same hazard.
func TestApplySandbox_BasicWithLauncher_DoesNotInjectRlimits(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeBasic).WithAllowMissingSandbox(true)
	r.WithResourceLimits(64*1024*1024, 8)
	p := &plugin.Plugin{ID: "themer", InstallDir: "/data/plugins/themer"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/themer/main.js")

	_, _, err := r.applySandbox(context.Background(), p, cmd)
	require.NoError(t, err)

	assertNoRlimitFlags(t, cmd.Args)
}

// TestResourceMonitor_StillCarriesTheQuotas pins the replacement contract: the
// caps are not lost, they live in the userspace sweep. Without this, dropping
// the rlimit injection could be mistaken for dropping the quota itself.
func TestResourceMonitor_StillCarriesTheQuotas(t *testing.T) {
	r := NewRunner()
	r.WithResourceLimits(123*1024*1024, 42)

	lim, _, _ := r.limitsSnapshot()
	cfg := lim.defaults()
	assert.Equal(t, int64(123*1024*1024), cfg.memBytes, "RSS quota must survive for checkResources")
	assert.Equal(t, 42, cfg.pidsMax, "PID quota must survive for checkResources")
}
