//go:build linux

package pluginrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// pluginSignalTimeout bounds one launcher round trip.
const pluginSignalTimeout = 5 * time.Second

// signalPlugin sends sig to the process group led by pid, then to the leader
// alone. A plugin under its own uid (root server, uid separation) is out of
// reach without CAP_KILL, which a non-root server never holds: on EPERM the
// launcher, which can take the plugin uid, sends it. nil when delivered or
// when the process is already gone.
func signalPlugin(ctx context.Context, pid int, sig syscall.Signal) error {
	// kill(-1) and kill(0) would hit every process we can reach or our own
	// group: a non-positive pid never reaches the kernel.
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, sig)
	if err != nil {
		err = syscall.Kill(pid, sig)
	}
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if !errors.Is(err, syscall.EPERM) {
		return err
	}
	return signalAsPluginUID(ctx, pid, sig)
}

// signalAsPluginUID runs `evey-plugin-launcher signal` under the plugin uid.
// Only ever for a process that IS of the plugin uid: anything else keeps the
// EPERM the kernel gave us.
func signalAsPluginUID(ctx context.Context, pid int, sig syscall.Signal) error {
	owner, err := procUID(pid)
	if err != nil {
		return nil // gone between the kill and the lookup
	}
	if owner != pluginUID {
		return syscall.EPERM
	}
	name, ok := map[syscall.Signal]string{syscall.SIGTERM: "TERM", syscall.SIGKILL: "KILL"}[sig]
	if !ok {
		return fmt.Errorf("signal %v: the launcher only relays TERM and KILL", sig)
	}
	launcher, err := launcherPath()
	if err != nil {
		return fmt.Errorf("signal plugin %d as its uid: %w", pid, err)
	}
	// A root server sets the identity itself; otherwise the launcher's
	// cap_setuid file capability does (the same two paths as the spawn).
	gid := os.Getgid()
	var attr *syscall.SysProcAttr
	if os.Getuid() == 0 {
		gid = pluginGID
		attr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: pluginUID, Gid: pluginGID}}
	}
	ctx, cancel := context.WithTimeout(ctx, pluginSignalTimeout)
	defer cancel()
	//nolint:gosec // G204: fixed binary (launcherPath), integer arguments only
	cmd := exec.CommandContext(ctx, launcher, "signal",
		"-uid", strconv.Itoa(pluginUID), "-gid", strconv.Itoa(gid), "-pid", strconv.Itoa(pid), "-sig", name)
	cmd.SysProcAttr = attr
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("signal plugin %d as its uid: %w: %s", pid, err, out)
	}
	return nil
}
