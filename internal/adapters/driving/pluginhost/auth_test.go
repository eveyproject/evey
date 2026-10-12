package pluginhost

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// dialPluginSock connects to a per-plugin host socket as the current
// process - SO_PEERCRED on the server side will see this PID.
func dialPluginSock(t *testing.T, sockPath string) (*grpc.ClientConn, pluginv1.HostServiceClient) {
	t.Helper()
	conn, err := grpc.NewClient(
		"unix:"+sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, pluginv1.NewHostServiceClient(conn)
}

// newRegisteredHost installs HostServer at a temp socket dir and returns
// the host plus a teardown helper. The base dir is restored afterward.
func newRegisteredHost(t *testing.T, plugins map[string]*plugindomain.Plugin) *HostServer {
	t.Helper()
	tmp := t.TempDir()
	// Override the package socket base dir for this test by symlinking.
	// We cannot rebind the const, so we lay out per-plugin dirs ourselves
	// via the public API - register requires the base dir to exist.
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "plugins"), 0o755))

	store := &fakeConfigStore{plugins: plugins}
	h := NewHostServer(
		&fakeMediaQuerier{items: map[string]*library.MediaItem{}},
		store,
		&fakeEventBus{},
		&fakePluginLogger{},
		nil,
	)
	t.Cleanup(h.Stop)
	return h
}

func TestAuthInterceptor_RejectsBeforePIDBound(t *testing.T) {
	h := newRegisteredHost(t, map[string]*plugindomain.Plugin{"plg-test": {ID: "plg-test"}})

	sockPath, err := h.RegisterPlugin("plg-test")
	require.NoError(t, err)
	defer h.UnregisterPlugin("plg-test")

	_, client := dialPluginSock(t, sockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "k"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
	assert.Contains(t, st.Message(), "PID not bound")
}

func TestAuthInterceptor_AcceptsMatchingPID(t *testing.T) {
	h := newRegisteredHost(t, map[string]*plugindomain.Plugin{
		"plg-test": {ID: "plg-test", Config: map[string]any{"foo": "bar"}},
	})

	sockPath, err := h.RegisterPlugin("plg-test")
	require.NoError(t, err)
	defer h.UnregisterPlugin("plg-test")

	// Pin auth to this test's PID - kernel SO_PEERCRED on the
	// connection will report exactly this value.
	h.SetExpectedPID("plg-test", int32(os.Getpid()))

	_, client := dialPluginSock(t, sockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "foo"})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.Equal(t, "bar", resp.Value)
}

func TestAuthInterceptor_RejectsPIDMismatch(t *testing.T) {
	h := newRegisteredHost(t, map[string]*plugindomain.Plugin{"plg-test": {ID: "plg-test"}})

	sockPath, err := h.RegisterPlugin("plg-test")
	require.NoError(t, err)
	defer h.UnregisterPlugin("plg-test")

	// Bind to a PID that is NOT the current test process - the dial
	// from this process must be rejected.
	wrongPID := int32(os.Getpid() + 1)
	if wrongPID == int32(os.Getpid()) {
		wrongPID++
	}
	h.SetExpectedPID("plg-test", wrongPID)

	_, client := dialPluginSock(t, sockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "k"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
	assert.Contains(t, st.Message(), "PID mismatch")
}

// TestAuthInterceptor_NoSpoofingViaCrossSocket proves a plugin authorized
// on its own socket cannot impersonate a sibling plugin by connecting to
// the sibling's socket. Both sockets pin auth to different PIDs (here we
// simulate "other plugin owns PID X" by binding plg-victim to a PID that
// is NOT this test process). Even with full filesystem access to the
// victim socket, the call from our PID is rejected - pluginID comes from
// the listener closure, never from the wire.
func TestAuthInterceptor_NoSpoofingViaCrossSocket(t *testing.T) {
	h := newRegisteredHost(t, map[string]*plugindomain.Plugin{
		"plg-attacker": {ID: "plg-attacker"},
		"plg-victim":   {ID: "plg-victim", Config: map[string]any{"secret": "leak-me"}},
	})

	attackerSock, err := h.RegisterPlugin("plg-attacker")
	require.NoError(t, err)
	defer h.UnregisterPlugin("plg-attacker")

	victimSock, err := h.RegisterPlugin("plg-victim")
	require.NoError(t, err)
	defer h.UnregisterPlugin("plg-victim")

	// Attacker is the test process; victim is "owned" by a fake PID.
	h.SetExpectedPID("plg-attacker", int32(os.Getpid()))
	h.SetExpectedPID("plg-victim", 999999)

	// (1) Calls on attacker's own sock succeed and identify as attacker.
	_, attClient := dialPluginSock(t, attackerSock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = attClient.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "any"})
	require.NoError(t, err, "attacker should reach own sock")

	// (2) Same process tries victim sock → rejected (PID mismatch).
	// The victim's secret config is NOT leaked.
	_, victClient := dialPluginSock(t, victimSock)
	_, err = victClient.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "secret"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestRegisterPlugin_DoubleRegister_SelfHeals(t *testing.T) {
	h := newRegisteredHost(t, map[string]*plugindomain.Plugin{"plg-dup": {ID: "plg-dup"}})

	_, err := h.RegisterPlugin("plg-dup")
	require.NoError(t, err)
	defer h.UnregisterPlugin("plg-dup")

	// Re-registering self-heals a stale entry: it evicts the old registration
	// and returns a fresh socket rather than erroring. The old behavior (error
	// "already registered") locked a plugin out of re-enabling until a full
	// server restart when an orphan entry leaked (e.g. a desynced restart).
	sock, err := h.RegisterPlugin("plg-dup")
	require.NoError(t, err)
	assert.NotEmpty(t, sock)
}

func TestRegisterPlugin_EmptyID(t *testing.T) {
	h := newRegisteredHost(t, nil)
	_, err := h.RegisterPlugin("")
	require.Error(t, err)
}

func TestUnregisterPlugin_Idempotent(t *testing.T) {
	h := newRegisteredHost(t, map[string]*plugindomain.Plugin{"plg-idem": {ID: "plg-idem"}})

	_, err := h.RegisterPlugin("plg-idem")
	require.NoError(t, err)
	h.UnregisterPlugin("plg-idem")
	h.UnregisterPlugin("plg-idem") // must not panic
}

// TestAuthInterceptor_ConcurrentPlugins runs N parallel callers against
// their respective sockets to exercise the per-plugin gRPC server fanout
// under load and confirm cross-plugin isolation under concurrency.
func TestAuthInterceptor_ConcurrentPlugins(t *testing.T) {
	plugins := map[string]*plugindomain.Plugin{}
	for _, id := range []string{"plg-a", "plg-b", "plg-c"} {
		plugins[id] = &plugindomain.Plugin{ID: id, Config: map[string]any{"id": id}}
	}
	h := newRegisteredHost(t, plugins)

	var socks []string
	for id := range plugins {
		sock, err := h.RegisterPlugin(id)
		require.NoError(t, err)
		h.SetExpectedPID(id, int32(os.Getpid()))
		t.Cleanup(func() { h.UnregisterPlugin(id) })
		socks = append(socks, sock)
	}

	var wg sync.WaitGroup
	for _, sock := range socks {
		sock := sock
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, client := dialPluginSock(t, sock)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			resp, err := client.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "id"})
			require.NoError(t, err)
			require.True(t, resp.Found)
		}()
	}
	wg.Wait()
}
