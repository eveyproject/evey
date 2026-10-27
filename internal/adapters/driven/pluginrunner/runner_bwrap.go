package pluginrunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// bwrapInfoTimeout bounds the wait for bwrap to report its child's PID. It
// writes it right after the clone, so only a wedged bwrap ever gets near it.
const bwrapInfoTimeout = 5 * time.Second

// buildBwrapCmd returns the bwrap argv prefix to wrap a plugin command.
// pluginDir is the writable bind mount for the plugin's own directory.
// pluginSockDir is the per-plugin sock dir on the host; it is bind-mounted
// (read-write) into the sandbox at the same path so the plugin can bind
// EVEY_PLUGIN_SOCK and the host (outside the sandbox) sees the socket.
// The network namespace is ALWAYS unshared: plugins have no direct egress.
// A plugin that needs the network reaches it only through the host Fetch
// broker over its Unix host socket (AF_UNIX is unaffected by --unshare-net),
// which enforces the external_fetch allow-list and an SSRF-safe dialer. This
// is why no /etc/hosts whitelist is bind-mounted here anymore.
// Returns nil when mode == "basic" so callers can short-circuit.
func buildBwrapCmd(pluginDir, pluginSockDir, mode string) []string {
	if mode == sandboxModeBasic {
		return nil
	}

	args := []string{
		"bwrap",
		"--unshare-user",
		"--unshare-pid",
		// No reaper between bwrap and the plugin: the process bwrap reports on
		// --info-fd is then the one that runs the plugin code, the PID the host
		// socket authenticates (SO_PEERCRED) and the resource monitor samples.
		"--as-pid-1",
		"--unshare-uts",
		"--unshare-ipc",
		"--unshare-cgroup",
		"--unshare-net",
		// No --disable-userns: it writes /proc/sys, read-only in a container.
		// The launcher's seccomp filter refuses unshare and CLONE_NEW* instead.
	}

	// Selective read-only mounts: cover all supported plugin runtimes
	// (bash, lua, nodejs, python, php) on Alpine and Debian without exposing
	// secrets-bearing host paths like /data, /var, /home, /root, /etc/passwd,
	// /etc/shadow, /etc/ssh, or /run.
	// --ro-bind-try silently skips paths absent on the current distro.
	args = append(args,
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind", "/sbin", "/sbin",
		"--ro-bind-try", "/lib", "/lib",
		"--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind-try", "/etc/ssl", "/etc/ssl",
		"--ro-bind-try", "/etc/ca-certificates", "/etc/ca-certificates",
		"--ro-bind-try", "/etc/resolv.conf", "/etc/resolv.conf",
		"--ro-bind-try", "/etc/nsswitch.conf", "/etc/nsswitch.conf",
		"--ro-bind-try", "/etc/alpine-release", "/etc/alpine-release",
		"--ro-bind-try", "/etc/os-release", "/etc/os-release",
		"--bind", pluginDir, pluginDir,
		"--proc", "/proc",
		// --dev synthesises a minimal /dev (null, zero, full, random, urandom,
		// pts) without exposing physical host devices like --dev-bind would.
		"--dev", "/dev",
		// --dev binds the host /dev/tty, the server's terminal (TIOCSTI). A
		// read-only bind is nodev, so the masked node cannot even be opened.
		"--ro-bind", "/dev/null", "/dev/tty",
		"--tmpfs", "/tmp",
	)

	// The sock dir lives under /tmp on the host (sockDir = "/tmp/evey/plugins").
	// --tmpfs /tmp above wipes any host-prepared directory inside the sandbox,
	// so we re-bind the sock dir RW AFTER the tmpfs to make the host-side sock
	// path visible to the plugin process.
	if pluginSockDir != "" {
		args = append(args, "--bind", pluginSockDir, pluginSockDir)
	}

	// No capability inside the user namespace. No --new-session: setsid would
	// leave the process group that Stop, Kill and the boot sweep signal.
	args = append(args,
		"--cap-drop", "ALL",
		"--die-with-parent",
		"--",
	)

	return args
}

// withBwrapInfoFD inserts --info-fd before the "--" that ends a buildBwrapCmd
// prefix, so bwrap writes its {"child-pid": N} report on fd.
func withBwrapInfoFD(prefix []string, fd int) []string {
	sep := len(prefix) - 1
	out := make([]string, 0, len(prefix)+2)
	out = append(out, prefix[:sep]...)
	out = append(out, "--info-fd", fmt.Sprint(fd))
	return append(out, prefix[sep:]...)
}

// withBwrapLauncher binds the launcher read-only at its own path before the
// "--" that ends a buildBwrapCmd prefix: it lives outside the bound system dirs.
func withBwrapLauncher(prefix []string, launcher string) []string {
	sep := len(prefix) - 1
	out := make([]string, 0, len(prefix)+3)
	out = append(out, prefix[:sep]...)
	out = append(out, "--ro-bind", launcher, launcher)
	return append(out, prefix[sep:]...)
}

// readBwrapChildPID reads the PID of the sandboxed child from bwrap's
// --info-fd report, in the server's PID namespace (the one SO_PEERCRED and
// /proc speak). An EOF means bwrap died before cloning (setup refused).
func readBwrapChildPID(info *os.File) (int, error) {
	defer info.Close()
	if err := info.SetReadDeadline(time.Now().Add(bwrapInfoTimeout)); err != nil {
		return 0, fmt.Errorf("bwrap info deadline: %w", err)
	}
	var report struct {
		ChildPID int `json:"child-pid"`
	}
	if err := json.NewDecoder(info).Decode(&report); err != nil {
		return 0, fmt.Errorf("read bwrap info: %w", err)
	}
	if report.ChildPID <= 0 {
		return 0, errors.New("bwrap info carries no child-pid")
	}
	return report.ChildPID, nil
}
