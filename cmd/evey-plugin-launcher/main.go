// Command evey-plugin-launcher is a tiny pre-exec shim that confines a plugin
// process with Landlock (filesystem allowlist) and optional POSIX rlimits,
// then execve's the plugin. The Evey plugin runner prepends it to the plugin
// argv when sandbox_mode=landlock (or when sandbox_mode=auto selects Landlock):
//
//	evey-plugin-launcher -ro /usr -ro /lib -rw <plugindir> -rw <sockdir> \
//	    -require-landlock -- /usr/bin/node /data/plugins/foo/main.js
//
// It is intentionally dependency-free (only golang.org/x/sys/unix, already a
// transitive dep) and self-contained so it can be audited at a glance: the
// whole security model is "deny everything, allow the listed directories".
//
// UID drop is handled by the parent (exec.Cmd SysProcAttr.Credential), so the
// launcher already runs as the unprivileged plugin user - Landlock needs no
// privilege by design. Landlock applies to the calling thread and is preserved
// across execve, so we lock the OS thread, restrict it, then exec on it.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	if v != "" {
		*s = append(*s, v)
	}
	return nil
}

type portList []uint64

func (p *portList) String() string {
	parts := make([]string, len(*p))
	for i, v := range *p {
		parts[i] = strconv.FormatUint(v, 10)
	}
	return strings.Join(parts, ",")
}
func (p *portList) Set(v string) error {
	n, err := strconv.ParseUint(v, 10, 16)
	if err != nil || n == 0 {
		return fmt.Errorf("invalid TCP port %q", v)
	}
	*p = append(*p, n)
	return nil
}

type config struct {
	roPaths         []string
	rwPaths         []string
	devPaths        []string // individual device nodes (/dev/null…) allowed read+write, skipped if missing
	renderDevPath   string   // GPU DRM render node (/dev/dri/renderD12N): read+write+IOCTL_DEV - the hardware.gpu grant, never a card node
	rlimitAS        uint64   // RLIMIT_AS bytes (virtual mem); risky for node, off by default
	rlimitNPROC     uint64   // RLIMIT_NPROC; note: per-UID, leaks across plugins sharing the UID
	rlimitCPU       uint64   // RLIMIT_CPU seconds
	rlimitNOFILE    uint64   // RLIMIT_NOFILE open fds
	requireLandlock bool     // fail closed if Landlock cannot be enforced

	restrictNet     bool     // govern TCP access (Landlock ABI v4+); deny-by-default
	netConnectPorts []uint64 // TCP ports the plugin may connect() to; empty = none

	seccomp  bool // install the kernel seccomp denylist before exec
	denyInet bool // also EPERM socket(AF_INET/AF_INET6) - no TCP/UDP egress
	denyFork bool // also EPERM process creation (threads stay allowed)
}

// installSeccomp is applySeccomp behind a seam so a test can make it fail.
var installSeccomp = applySeccomp

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	if len(argv) > 0 && argv[0] == "signal" {
		return runSignal(argv[1:])
	}
	// Captured before any setup so the pre-exec orphan check below can detect a
	// parent that died while the launcher was confining itself.
	startPPID := os.Getppid()

	// Everything after the first "--" is the command to exec; flags come before.
	sep := -1
	for i, a := range argv {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep < 0 {
		fmt.Fprintln(os.Stderr, "evey-plugin-launcher: missing '--' separator before the command")
		return 2
	}
	flagArgs, cmdArgs := argv[:sep], argv[sep+1:]
	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "evey-plugin-launcher: no command after '--'")
		return 2
	}

	var ro, rw, dev stringList
	var netConnect portList
	fs := flag.NewFlagSet("evey-plugin-launcher", flag.ContinueOnError)
	fs.Var(&ro, "ro", "directory to allow read+execute (repeatable)")
	fs.Var(&rw, "rw", "directory to allow read+write+create (repeatable)")
	fs.Var(&dev, "dev", "single device node to allow read+write, e.g. /dev/null (repeatable; skipped if missing). Use instead of -rw /dev so GPU/disk nodes stay denied.")
	renderDev := fs.String("render-dev", "", "GPU DRM render node (/dev/dri/renderD12N) to allow read+write+IOCTL_DEV - the hardware.gpu grant. Refused unless the path matches a render node; never a card/primary node.")
	fs.Var(&netConnect, "net-connect", "TCP port the plugin may connect to (repeatable; implies -restrict-net)")
	restrictNet := fs.Bool("restrict-net", false, "govern TCP access via Landlock v4: deny every bind/connect except the -net-connect ports")
	asLimit := fs.Uint64("rlimit-as", 0, "RLIMIT_AS bytes (0=unset)")
	nprocLimit := fs.Uint64("rlimit-nproc", 0, "RLIMIT_NPROC (0=unset)")
	cpuLimit := fs.Uint64("rlimit-cpu", 0, "RLIMIT_CPU seconds (0=unset)")
	nofileLimit := fs.Uint64("rlimit-nofile", 0, "RLIMIT_NOFILE (0=unset)")
	require := fs.Bool("require-landlock", false, "exit non-zero if Landlock cannot be enforced (fail closed)")
	seccompFlag := fs.Bool("seccomp", false, "install a kernel seccomp denylist (ptrace/mount/unshare/kill/…) before exec")
	denyInetFlag := fs.Bool("deny-inet", false, "also EPERM socket(AF_INET/AF_INET6): block all TCP/UDP egress (no network permission)")
	denyForkFlag := fs.Bool("deny-fork", false, "also EPERM process creation - fork/vfork and clone without CLONE_THREAD (threads keep working; use for plugins without the subprocess permission)")
	uidFlag := fs.Int("uid", 0, "setuid to this uid before exec (0 = no drop; needs the cap_setuid file capability or a root parent)")
	gidFlag := fs.Int("gid", 0, "setgid to this (shared) gid before exec; used with -uid")
	uidBestEffort := fs.Bool("uid-besteffort", false, "if the uid drop fails, warn and run as the server uid instead of refusing (the plugin stays confined by the other layers)")
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	cfg := config{
		roPaths:         ro,
		rwPaths:         rw,
		devPaths:        dev,
		renderDevPath:   *renderDev,
		rlimitAS:        *asLimit,
		rlimitNPROC:     *nprocLimit,
		rlimitCPU:       *cpuLimit,
		rlimitNOFILE:    *nofileLimit,
		requireLandlock: *require,
		restrictNet:     *restrictNet || len(netConnect) > 0,
		netConnectPorts: netConnect,
		seccomp:         *seccompFlag,
		denyInet:        *denyInetFlag,
		denyFork:        *denyForkFlag,
	}

	// Privilege drop FIRST: before any OS thread is locked and before seccomp
	// (which denies setuid) is installed. Fail closed - if a drop was requested
	// but is impossible (missing cap_setuid, no-new-privileges, or an unsafe
	// uid), refuse to run rather than start the plugin under the wrong identity.
	if err := dropPrivileges(*uidFlag, *gidFlag); err != nil {
		// A drop error means no setuid happened, so the process is still at the
		// server uid (no escalation). In best-effort mode we surface it loudly and
		// continue - the plugin runs same-uid, confined by the other layers - so a
		// host that can't grant the capability (or uses no-new-privileges) keeps
		// working instead of failing every plugin closed.
		if *uidBestEffort {
			fmt.Fprintf(os.Stderr, "evey-plugin-launcher: WARNING uid separation unavailable, running as the server uid: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "evey-plugin-launcher: privilege drop failed, refusing to run: %v\n", err)
			return 1
		}
	}

	// Resource caps next (best effort; failures are warnings, not fatal).
	applyRlimits(cfg)

	// Filesystem confinement. On a kernel without Landlock this fails; honour
	// the fail-closed contract so we never silently run a plugin unconfined.
	if err := applyLandlock(cfg); err != nil {
		if cfg.requireLandlock {
			fmt.Fprintf(os.Stderr, "evey-plugin-launcher: Landlock enforcement failed, refusing to run unconfined: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "evey-plugin-launcher: WARNING running without Landlock confinement: %v\n", err)
	}

	bin := cmdArgs[0]
	if !strings.Contains(bin, "/") {
		if p, lookErr := exec.LookPath(bin); lookErr == nil {
			bin = p
		}
	}

	// Syscall confinement, installed LAST (after path lookup, right before exec)
	// so it can't interfere with the launcher's own setup. Best-effort unless
	// -require-landlock: a kernel without seccomp then degrades to Landlock +
	// the LD_PRELOAD shim rather than refusing to run. The filter is inherited across execve into the plugin.
	if cfg.seccomp {
		if err := installSeccomp(cfg.denyInet, cfg.denyFork); err != nil {
			// -require-landlock is the fail-closed contract; the syscall layer
			// (fork, namespaces, IP sockets) is part of it, not a bonus.
			if cfg.requireLandlock {
				fmt.Fprintf(os.Stderr, "evey-plugin-launcher: seccomp filter not installed, refusing to run: %v\n", err)
				return 1
			}
			fmt.Fprintf(os.Stderr, "evey-plugin-launcher: WARNING seccomp filter not installed: %v\n", err)
		}
	}

	// Re-arm the parent-death signal on the exec'ing thread, LAST. The runner
	// arms PR_SET_PDEATHSIG via SysProcAttr, but the attribute is per-thread and
	// does not reliably survive this launcher (empirically: installing the
	// seccomp filter above loses it across execve; a plugin then outlives a dead
	// server and leaks ~25 MB of node per restart). prctl is not in the seccomp
	// denylist, so this works after the filter is installed; the attribute is
	// preserved across the execve below (the plugin binary carries no setuid/file
	// caps). runtime.LockOSThread pins us to the thread the confinement layers
	// restricted - the same one syscall.Exec replaces.
	runtime.LockOSThread()
	if err := armParentDeathSignal(); err != nil {
		fmt.Fprintf(os.Stderr, "evey-plugin-launcher: WARNING parent-death signal not armed: %v\n", err)
	} else if os.Getppid() != startPPID {
		// The server died while we were setting up: pdeathsig missed the window
		// (the attribute only fires on a FUTURE parent death). Refuse to start an
		// already-orphaned plugin rather than leak it.
		fmt.Fprintln(os.Stderr, "evey-plugin-launcher: parent exited during setup, refusing to start orphaned plugin")
		return 1
	}

	// syscall.Exec runs on the current (Landlock-restricted, OS-locked) thread;
	// execve replaces the image and the new program inherits the Landlock domain.
	if err := syscall.Exec(bin, cmdArgs, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "evey-plugin-launcher: exec %s: %v\n", bin, err)
		return 127
	}
	return 0 // unreachable on success
}
