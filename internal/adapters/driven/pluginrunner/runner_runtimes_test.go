package pluginrunner

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRuntimeLaunchers starts the two reference plugins whose runtime is not
// part of the server image but added to it (testdata/runtimes/Dockerfile): a
// .jar run by "java -jar plugin.jar" and a .dll run by "dotnet plugin.dll".
// Each must pass the install check and come up in every sandbox mode the host
// supports, far enough to bind its socket and write in its install dir.
// Skipped unless EVEY_TEST_RUNTIME_FIXTURES names the fixtures that image
// holds (make plugin-runtimes-itest, run by CI). There, Landlock and basic
// must run; bubblewrap and strict run where the host allows user namespaces.
func TestRuntimeLaunchers(t *testing.T) {
	fixtures := os.Getenv("EVEY_TEST_RUNTIME_FIXTURES")
	if fixtures == "" {
		t.Skip("EVEY_TEST_RUNTIME_FIXTURES unset (make plugin-runtimes-itest)")
	}
	cases := []struct {
		name, entrypoint, marker string
	}{
		{"java", "java -jar plugin.jar", "java "},
		{"dotnet", "dotnet plugin.dll", "dotnet "},
	}
	for _, mode := range []string{sandboxModeLandlock, sandboxModeBubblewrap, sandboxModeStrict, sandboxModeBasic} {
		for _, c := range cases {
			t.Run(mode+"/"+c.name, func(t *testing.T) {
				if err := ProbeSandbox(mode); err != nil {
					if mode != sandboxModeBubblewrap && mode != sandboxModeStrict {
						t.Fatalf("%s unusable here: %v", mode, err)
					}
					t.Skipf("%s unusable here: %v", mode, err)
				}
				src := filepath.Join(fixtures, c.name)
				require.NoError(t, NewRuntimeAvailability("/data/plugins").
					CheckEntrypoint(zipDir(t, src), c.name, c.entrypoint))

				// Not under /tmp: the bwrap sandbox mounts a fresh tmpfs there.
				wd, err := os.Getwd()
				require.NoError(t, err)
				pluginsDir, err := os.MkdirTemp(wd, "evey-runtime-")
				require.NoError(t, err)
				t.Cleanup(func() { _ = os.RemoveAll(pluginsDir) })
				require.NoError(t, os.Chmod(pluginsDir, 0o755)) //nolint:gosec // the plugin must traverse it
				id := "plg-rt-" + c.name
				installDir := filepath.Join(pluginsDir, id)
				require.NoError(t, os.CopyFS(installDir, os.DirFS(src)))

				r := NewRunner(pluginsDir).WithSandboxMode(mode)
				p := &plugin.Plugin{ID: id, Name: c.name, Entrypoint: c.entrypoint, InstallDir: installDir}
				require.NoError(t, r.Start(context.Background(), p))
				t.Cleanup(func() { _ = r.Kill(id) })

				var report string
				reported := assert.Eventually(t, func() bool {
					b, readErr := os.ReadFile(filepath.Join(installDir, "started.txt"))
					report = string(b)
					return readErr == nil && strings.HasPrefix(report, c.marker)
				}, 30*time.Second, 100*time.Millisecond)
				if !reported {
					t.Fatalf("the plugin never reported; it printed: %s", crashTail(r, id))
				}
				t.Logf("%s: %s", mode, strings.TrimSpace(report))
				// The plugin read its own /proc/self/maps: the seccomp lib is loaded
				// in the runtime itself in the modes that preload it (in strict,
				// through bwrap's --setenv).
				require.Contains(t, report, "seccomp_lib="+strconv.FormatBool(mode != sandboxModeBubblewrap), mode)
				// The runtime sized its heap from the plugin's memory cap, not the host's RAM.
				_, heap, ok := strings.Cut(strings.TrimSpace(report), "max_heap=")
				require.True(t, ok, report)
				heapBytes, err := strconv.ParseInt(heap, 10, 64)
				require.NoError(t, err)
				require.LessOrEqual(t, heapBytes, defaultMemBytes, "heap above the plugin memory cap")
			})
		}
	}
}

// crashTail returns what the plugin printed, if it is still tracked.
func crashTail(r *Runner, id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e := r.processes[id]; e != nil && e.tail != nil {
		return e.tail.String()
	}
	return "(process gone)"
}

// zipDir packs a plugin directory the way it is distributed.
func zipDir(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "plugin.zip")
	f, err := os.Create(out)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	require.NoError(t, zw.AddFS(os.DirFS(dir)))
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return out
}
