package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildCrashingBinary builds a plugin that writes a marker to stderr then exits
// 1 immediately - the shape of a node plugin missing a module, or bwrap failing
// its sandbox setup.
func buildCrashingBinary(t *testing.T, stderrMsg string) string {
	t.Helper()
	return buildExitingBinary(t, stderrMsg, 1)
}

// buildExitingBinary is buildCrashingBinary with the exit status chosen.
func buildExitingBinary(t *testing.T, stderrMsg string, status int) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "test-plugin")
	require.NoError(t, os.Chmod(dir, 0o755))
	require.NoError(t, os.Chmod(filepath.Dir(dir), 0o755))

	prog := "package main\n" +
		"import (\n\t\"fmt\"\n\t\"os\"\n)\n" +
		"func main() {\n" +
		"\tfmt.Fprintln(os.Stderr, " + quote(stderrMsg) + ")\n" +
		"\tos.Exit(" + strconv.Itoa(status) + ")\n" +
		"}\n"
	require.NoError(t, os.WriteFile(src, []byte(prog), 0o644))

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))
	return bin
}

func quote(s string) string { return "`" + s + "`" }

// Regression: when a plugin process crashes before its socket comes up, the
// reported error must carry the process's own stderr (the real cause), not a
// bare "wait for plugin socket: timeout". This is what made the weather plugin
// failure ("Cannot find module '@grpc/grpc-js'") invisible to the admin.
func TestRunner_CrashStderrSurfacedOverSocketTimeout(t *testing.T) {
	const marker = "FATAL_MARKER: cannot find module grpc-js"
	bin := buildCrashingBinary(t, marker)
	dir := filepath.Dir(bin)

	resultCh := make(chan error, 1)
	r := NewRunner(dir).
		WithAllowMissingSandbox(true).
		WithIPCCallback(func(_ string, err error) { resultCh <- err })

	p := &plugin.Plugin{ID: "plg-crash-1", Name: bin}
	require.NoError(t, r.Start(context.Background(), p))

	select {
	case err := <-resultCh:
		require.Error(t, err, "a crashing plugin must report an error")
		assert.Contains(t, err.Error(), marker,
			"reported error must include the plugin's stderr tail")
		assert.Contains(t, err.Error(), "exited before becoming ready",
			"must surface the crash, not a bare socket timeout")
	case <-time.After(20 * time.Second):
		t.Fatal("no IPC result within 20s")
	}
}

// A plugin that exits with status 0 before binding (a misread config it treats
// as "nothing to do") is just as dead as one that crashed: the admin must get
// its output, not a bare "exited before binding <socket path>".
func TestRunner_CleanExitBeforeReadySurfacesOutput(t *testing.T) {
	const marker = "NOTHING_TO_DO: config key api_key is empty"
	bin := buildExitingBinary(t, marker, 0)
	dir := filepath.Dir(bin)

	resultCh := make(chan error, 1)
	r := NewRunner(dir).
		WithAllowMissingSandbox(true).
		WithIPCCallback(func(_ string, err error) { resultCh <- err })

	p := &plugin.Plugin{ID: "plg-clean-exit-1", Name: bin}
	require.NoError(t, r.Start(context.Background(), p))

	select {
	case err := <-resultCh:
		require.Error(t, err)
		assert.Contains(t, err.Error(), marker, "reported error must include the plugin's output")
		assert.Contains(t, err.Error(), "exited before becoming ready")
	case <-time.After(20 * time.Second):
		t.Fatal("no IPC result within 20s")
	}
}
