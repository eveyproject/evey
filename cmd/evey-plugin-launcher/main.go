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
// launcher already runs as the unprivileged plugin user — Landlock needs no
// privilege by design. Landlock applies to the calling thread and is preserved
// across execve, so we lock the OS thread, restrict it, then exec on it.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
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

type config struct {
	roPaths         []string
	rwPaths         []string
	rlimitAS        uint64 // RLIMIT_AS bytes (virtual mem); risky for node, off by default
	rlimitNPROC     uint64 // RLIMIT_NPROC; note: per-UID, leaks across plugins sharing the UID
	rlimitCPU       uint64 // RLIMIT_CPU seconds
	rlimitNOFILE    uint64 // RLIMIT_NOFILE open fds
	requireLandlock bool   // fail closed if Landlock cannot be enforced
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
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

	var ro, rw stringList
	fs := flag.NewFlagSet("evey-plugin-launcher", flag.ContinueOnError)
	fs.Var(&ro, "ro", "directory to allow read+execute (repeatable)")
	fs.Var(&rw, "rw", "directory to allow read+write+create (repeatable)")
	asLimit := fs.Uint64("rlimit-as", 0, "RLIMIT_AS bytes (0=unset)")
	nprocLimit := fs.Uint64("rlimit-nproc", 0, "RLIMIT_NPROC (0=unset)")
	cpuLimit := fs.Uint64("rlimit-cpu", 0, "RLIMIT_CPU seconds (0=unset)")
	nofileLimit := fs.Uint64("rlimit-nofile", 0, "RLIMIT_NOFILE (0=unset)")
	require := fs.Bool("require-landlock", false, "exit non-zero if Landlock cannot be enforced (fail closed)")
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	cfg := config{
		roPaths:         ro,
		rwPaths:         rw,
		rlimitAS:        *asLimit,
		rlimitNPROC:     *nprocLimit,
		rlimitCPU:       *cpuLimit,
		rlimitNOFILE:    *nofileLimit,
		requireLandlock: *require,
	}

	// Resource caps first (best effort; failures are warnings, not fatal).
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
	// syscall.Exec runs on the current (Landlock-restricted, OS-locked) thread;
	// execve replaces the image and the new program inherits the Landlock domain.
	if err := syscall.Exec(bin, cmdArgs, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "evey-plugin-launcher: exec %s: %v\n", bin, err)
		return 127
	}
	return 0 // unreachable on success
}
