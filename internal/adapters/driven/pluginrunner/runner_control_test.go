//go:build linux

package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signalPluginGroup uses kill(-pid) to reach a plugin's whole process group. A
// non-positive pid is catastrophic there: kill(0) hits the caller's OWN group
// and kill(-1) hits every process it can signal - either would take the server
// down. The guard must make those a no-op. If this regresses, the negative-pid
// call below would SIGKILL this test's own process group and the run dies.
func TestSignalPluginGroup_NonPositivePidIsNoop(t *testing.T) {
	for _, pid := range []int{0, -1} {
		signalPluginGroup(context.Background(), pid, syscall.SIGKILL)
	}
	assert.True(t, true, "reached here => the guard stopped a self-directed kill")
}

// signalPluginGroup targets the process group, but a process not spawned as its
// own group leader (no Setpgid) has no pgid == pid, so kill(-pid) returns ESRCH.
// The fallback to a direct kill(pid) must still reach it. A plain `sleep` child
// started here is exactly that case.
func TestSignalPluginGroup_FallsBackToLeaderWhenNotGroupLeader(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	cmd := exec.Command("sleep", "120") // no SysProcAttr.Setpgid => shares our group
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	signalPluginGroup(context.Background(), pid, syscall.SIGKILL)

	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("signalPluginGroup did not reach a non-group-leader process via the leader-pid fallback")
	}
	// The group send (kill(-pid)) must NOT have hit our own group: if it had,
	// this process would already be dead. Reaching here proves containment.
	assert.NoError(t, syscall.Kill(os.Getpid(), 0), "test process must still be alive")
}
