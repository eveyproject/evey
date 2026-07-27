package pluginrunner

import (
	"context"
	"os/exec"
	"slices"
	"strconv"
	"testing"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplySandbox_Landlock_InjectsLauncherRlimits proves the launcher argv
// carries the SAME memory/PID caps the resource monitor enforces post-hoc
// (r.limitsSnapshot), so a plugin that allocates without bound now hits a
// kernel RLIMIT_AS failure immediately instead of only after the next ~2s
// sweep notices its RSS.
func TestApplySandbox_Landlock_InjectsLauncherRlimits(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeLandlock).WithAllowMissingSandbox(true)
	r.WithResourceLimits(123*1024*1024, 42)
	p := &plugin.Plugin{ID: "weather", InstallDir: "/data/plugins/weather"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")

	cleanup, err := r.applySandbox(context.Background(), p, cmd)
	require.NoError(t, err)
	if cleanup != nil {
		cleanup()
	}

	asIdx := slices.Index(cmd.Args, "-rlimit-as")
	require.GreaterOrEqual(t, asIdx, 0, "-rlimit-as must be present")
	assert.Equal(t, strconv.FormatInt(123*1024*1024, 10), cmd.Args[asIdx+1])

	nprocIdx := slices.Index(cmd.Args, "-rlimit-nproc")
	require.GreaterOrEqual(t, nprocIdx, 0, "-rlimit-nproc must be present")
	assert.Equal(t, "42", cmd.Args[nprocIdx+1])

	sep := slices.Index(cmd.Args, "--")
	require.GreaterOrEqual(t, sep, 0)
	assert.Less(t, asIdx, sep, "-rlimit-as must precede the -- separator (it is a launcher flag)")
	assert.Less(t, nprocIdx, sep, "-rlimit-nproc must precede the -- separator (it is a launcher flag)")

	// The original command stays intact at the tail.
	assert.Equal(t, "/usr/bin/node", cmd.Args[len(cmd.Args)-2])
	assert.Equal(t, "/data/plugins/weather/main.js", cmd.Args[len(cmd.Args)-1])
}

// TestApplySandbox_BasicWithLauncher_InjectsRlimits covers the other launcher-
// backed path (sandbox_mode=basic, launcher available): same caps must apply.
func TestApplySandbox_BasicWithLauncher_InjectsRlimits(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeBasic).WithAllowMissingSandbox(true)
	r.WithResourceLimits(64*1024*1024, 8)
	p := &plugin.Plugin{ID: "themer", InstallDir: "/data/plugins/themer"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/themer/main.js")

	_, err := r.applySandbox(context.Background(), p, cmd)
	require.NoError(t, err)

	asIdx := slices.Index(cmd.Args, "-rlimit-as")
	require.GreaterOrEqual(t, asIdx, 0, "-rlimit-as must be present under basic+launcher too")
	assert.Equal(t, strconv.FormatInt(64*1024*1024, 10), cmd.Args[asIdx+1])
	nprocIdx := slices.Index(cmd.Args, "-rlimit-nproc")
	require.GreaterOrEqual(t, nprocIdx, 0)
	assert.Equal(t, "8", cmd.Args[nprocIdx+1])
}

// TestApplySandbox_Landlock_RlimitsUseConfiguredDefaults proves the injected
// values come from the SAME defaulted source checkResources() reads (no
// separate hardcoded constant): with no explicit WithResourceLimits call, the
// launcher must receive the package defaults (defaultMemBytes/defaultPIDsMax).
func TestApplySandbox_Landlock_RlimitsUseConfiguredDefaults(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeLandlock).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{ID: "weather", InstallDir: "/data/plugins/weather"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")

	_, err := r.applySandbox(context.Background(), p, cmd)
	require.NoError(t, err)

	asIdx := slices.Index(cmd.Args, "-rlimit-as")
	require.GreaterOrEqual(t, asIdx, 0)
	assert.Equal(t, strconv.FormatInt(defaultMemBytes, 10), cmd.Args[asIdx+1])
	nprocIdx := slices.Index(cmd.Args, "-rlimit-nproc")
	require.GreaterOrEqual(t, nprocIdx, 0)
	assert.Equal(t, strconv.Itoa(defaultPIDsMax), cmd.Args[nprocIdx+1])
}
