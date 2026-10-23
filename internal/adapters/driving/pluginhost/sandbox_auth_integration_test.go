package pluginhost

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// authProbeName is the file name the test binary is copied to inside the
// plugin dir. When the binary runs under that name it is the plugin, not the
// test suite: it calls one host RPC and records the outcome.
const authProbeName = "authprobe"

func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == authProbeName {
		os.Exit(runAuthProbe())
	}
	os.Exit(m.Run())
}

// runAuthProbe is the plugin side: it calls GetMediaItem over EVEY_HOST_SOCK
// until the host has bound its PID, then writes the final status next to
// itself for the test to read.
func runAuthProbe() int {
	out := filepath.Join(filepath.Dir(os.Args[0]), authProbeName+".out")
	conn, err := grpc.NewClient("unix:"+os.Getenv("EVEY_HOST_SOCK"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = os.WriteFile(out, []byte("dial: "+err.Error()), 0o600)
		return 1
	}
	defer conn.Close()
	client := pluginv1.NewHostServiceClient(conn)

	var st *status.Status
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err = client.GetMediaItem(ctx, &pluginv1.GetMediaItemRequest{Id: "m1"})
		cancel()
		st = status.Convert(err)
		if st.Code() == codes.Unavailable || st.Message() == "plugin PID not bound yet" {
			continue
		}
		break
	}
	_ = os.WriteFile(out, fmt.Appendf(nil, "%s|%s", st.Code(), st.Message()), 0o600)
	// Stay alive so the runner sees a running plugin until the test stops it.
	time.Sleep(time.Minute)
	return 0
}

// TestBwrapPluginAuthenticatesToHost launches a real process inside the real
// bubblewrap sandbox against the real per-plugin host socket. --unshare-pid
// makes the plugin a child of the bwrap process the runner started, so the
// SO_PEERCRED check must expect that child, not bwrap itself.
func TestBwrapPluginAuthenticatesToHost(t *testing.T) {
	if err := pluginrunner.ProbeSandbox("bubblewrap"); err != nil {
		t.Skipf("bubblewrap unusable here: %v", err)
	}
	assertSandboxedPluginAuthenticates(t, "bubblewrap")
}

// TestLandlockPluginAuthenticatesToHost is the same round trip under Landlock,
// where the launcher execs the plugin in place and cmd.Process is the plugin.
func TestLandlockPluginAuthenticatesToHost(t *testing.T) {
	if err := pluginrunner.ProbeSandbox("landlock"); err != nil {
		t.Skipf("landlock unusable here: %v", err)
	}
	if _, err := exec.LookPath("evey-plugin-launcher"); err != nil && os.Getenv("EVEY_PLUGIN_LAUNCHER") == "" {
		t.Skip("evey-plugin-launcher not found (set EVEY_PLUGIN_LAUNCHER)")
	}
	assertSandboxedPluginAuthenticates(t, "landlock")
}

func assertSandboxedPluginAuthenticates(t *testing.T, mode string) {
	t.Helper()

	// Not under /tmp: the sandbox mounts a fresh tmpfs there, which would hide
	// the install dir from the plugin (real plugins live under /data/plugins).
	wd, err := os.Getwd()
	require.NoError(t, err)
	pluginsDir, err := os.MkdirTemp(wd, "evey-sandbox-auth-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(pluginsDir) })
	installDir := filepath.Join(pluginsDir, "authprobe-plugin")
	require.NoError(t, os.MkdirAll(installDir, 0o755))
	require.NoError(t, os.Chmod(pluginsDir, 0o755)) //nolint:gosec // the plugin may run under evey-plugin and must traverse it
	probe := filepath.Join(installDir, authProbeName)
	copyExecutable(t, probe)
	if os.Getuid() == 0 {
		// Root drops the plugin to evey-plugin (1001): it must write its result.
		require.NoError(t, os.Chown(installDir, 1001, 1001))
	}

	h := newTestHostServer()
	t.Cleanup(h.Stop)
	r := pluginrunner.NewRunner(pluginsDir).
		WithAllowMissingSandbox(true).
		WithSandboxMode(mode).
		WithHostRegistry(h)

	p := &plugindomain.Plugin{ID: "auth-probe-" + mode, Name: authProbeName, Entrypoint: authProbeName, InstallDir: installDir}
	require.NoError(t, r.Start(context.Background(), p))
	t.Cleanup(func() { _ = r.Kill(p.ID) })

	var got string
	require.Eventually(t, func() bool {
		b, readErr := os.ReadFile(filepath.Join(installDir, authProbeName+".out"))
		got = string(b)
		return readErr == nil && got != ""
	}, 15*time.Second, 50*time.Millisecond, "the sandboxed plugin never reported its host RPC outcome")
	require.Equal(t, codes.OK.String()+"|", got, "host RPC from the sandboxed plugin must authenticate")
}

func copyExecutable(t *testing.T, dst string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	src, err := os.Open(self)
	require.NoError(t, err)
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) //nolint:gosec // test-owned path
	require.NoError(t, err)
	_, err = io.Copy(out, src)
	require.NoError(t, err)
	require.NoError(t, out.Close())
	require.NotContains(t, dst, "..")
}
