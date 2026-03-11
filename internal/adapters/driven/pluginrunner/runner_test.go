package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "test-plugin")

	err := os.WriteFile(src, []byte(`package main
import (
	"os"
	"os/signal"
	"syscall"
)
func main() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	<-ch
}
`), 0o644)
	require.NoError(t, err)

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))
	return bin
}

func TestRunnerStartStop(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()

	r := NewRunner(dir)

	p := &plugin.Plugin{
		ID:   "plg-test-1",
		Name: bin,
	}

	err := r.Start(ctx, p)
	require.NoError(t, err)

	healthy, err := r.Health(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, healthy)

	err = r.Stop(ctx, p.ID)
	require.NoError(t, err)

	healthy, err = r.Health(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestRunnerStopNotRunning(t *testing.T) {
	r := NewRunner()
	err := r.Stop(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestRunnerSendEventNotRunning(t *testing.T) {
	r := NewRunner()
	err := r.SendEvent(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
}

func TestRunnerHealthNotRunning(t *testing.T) {
	r := NewRunner()
	healthy, err := r.Health(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestRunnerStartCreatesSocketDir(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()
	r := NewRunner(dir)

	p := &plugin.Plugin{
		ID:   "plg-sockdir-test",
		Name: bin,
	}

	err := r.Start(ctx, p)
	require.NoError(t, err)
	defer r.Stop(ctx, p.ID)

	time.Sleep(50 * time.Millisecond)

	_, err = os.Stat(sockDir)
	assert.NoError(t, err)
}

func TestRunnerRejectsEntrypointOutsidePluginDir(t *testing.T) {
	r := NewRunner("/data/plugins")
	ctx := context.Background()

	p := &plugin.Plugin{
		ID:         "plg-bad",
		Entrypoint: "/usr/bin/ls",
	}

	err := r.Start(ctx, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside allowed plugin directories")
}

func TestRunnerRejectsPathTraversal(t *testing.T) {
	r := NewRunner("/data/plugins")
	ctx := context.Background()

	p := &plugin.Plugin{
		ID:         "plg-traversal",
		Entrypoint: "../../../etc/passwd",
		InstallDir: "/data/plugins/evil",
	}

	err := r.Start(ctx, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not contain '..'")
}
