package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "test-plugin")
	require.NoError(t, os.Chmod(dir, 0o755))
	require.NoError(t, os.Chmod(filepath.Dir(dir), 0o755))

	prog := "package main\n" +
		"import (\n\t\"fmt\"\n\t\"os\"\n)\n" +
		"func main() {\n" +
		"\tfmt.Fprintln(os.Stderr, " + quote(stderrMsg) + ")\n" +
		"\tos.Exit(1)\n" +
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
