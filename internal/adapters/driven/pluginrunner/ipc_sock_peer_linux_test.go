//go:build linux

package pluginrunner

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestPluginSockPeerIsTheChildProcess runs the listener in a real child
// process, as a plugin is: its pid passes the SO_PEERCRED check, the server's
// own pid (a stand-in for a neighbour plugin) does not.
func TestPluginSockPeerIsTheChildProcess(t *testing.T) {
	if sock := os.Getenv("PLUGIN_SOCK_CHILD"); sock != "" {
		l, err := net.Listen("unix", sock)
		if err != nil {
			os.Exit(3)
		}
		for {
			c, err := l.Accept()
			if err != nil {
				os.Exit(0)
			}
			c.Close()
		}
	}
	sock := filepath.Join(t.TempDir(), "plugin.sock")
	cmd := exec.Command(os.Args[0], "-test.run=^TestPluginSockPeerIsTheChildProcess$")
	cmd.Env = append(os.Environ(), "PLUGIN_SOCK_CHILD="+sock)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := waitForSocket(ctx, sock, make(chan struct{}), cmd.Process.Pid); err != nil {
		t.Fatalf("the plugin process listens: %v", err)
	}
	c, err := newIPCClient(sock, make(chan struct{}), cmd.Process.Pid)
	if err != nil {
		t.Fatalf("newIPCClient: %v", err)
	}
	_ = c.Close()
	if err := waitForSocket(ctx, sock, make(chan struct{}), os.Getpid()); !errors.Is(err, errSockImpostor) {
		t.Fatalf("wrong expected pid: %v, want errSockImpostor", err)
	}
}
