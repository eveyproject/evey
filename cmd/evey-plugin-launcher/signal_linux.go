//go:build linux

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// runSignal is the `signal` subcommand. A plugin under its own uid (root
// server, uid separation) cannot be signalled by a server without CAP_KILL,
// and a non-root server never has it: Stop's escalation, Kill and the boot
// sweep of orphans all failed with EPERM, leaking one plugin per restart. The
// launcher can take the plugin uid, so the server asks it instead.
//
// Exit codes: 0 sent (or the target is already gone), 1 drop/signal failure,
// 2 bad arguments, 3 target not owned by that uid.
func runSignal(argv []string) int {
	fs := flag.NewFlagSet("evey-plugin-launcher signal", flag.ContinueOnError)
	uid := fs.Int("uid", 0, "plugin uid to take before signalling (>= 1000)")
	gid := fs.Int("gid", 0, "gid to take with -uid")
	pid := fs.Int("pid", 0, "plugin process (group leader) to signal")
	sigName := fs.String("sig", "", "TERM or KILL")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	sig, ok := map[string]syscall.Signal{"TERM": syscall.SIGTERM, "KILL": syscall.SIGKILL}[*sigName]
	if !ok || *pid <= 1 || *uid < 1000 || fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "evey-plugin-launcher signal: want -uid >= 1000 -gid -pid > 1 -sig TERM|KILL")
		return 2
	}
	if err := dropPrivileges(*uid, *gid); err != nil {
		fmt.Fprintf(os.Stderr, "evey-plugin-launcher signal: %v\n", err)
		return 1
	}
	// dropPrivileges is a no-op when already there: insist on the identity.
	if os.Getuid() != *uid || os.Geteuid() != *uid {
		fmt.Fprintf(os.Stderr, "evey-plugin-launcher signal: running as uid %d, not %d\n", os.Geteuid(), *uid)
		return 1
	}
	owner, err := procRealUID(*pid)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil || owner != *uid {
		fmt.Fprintf(os.Stderr, "evey-plugin-launcher signal: pid %d is not a process of uid %d\n", *pid, *uid)
		return 3
	}
	// The plugin's whole group (Setpgid at spawn), else the leader alone.
	if err := syscall.Kill(-*pid, sig); err != nil {
		if err := syscall.Kill(*pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			fmt.Fprintf(os.Stderr, "evey-plugin-launcher signal: kill(%d): %v\n", *pid, err)
			return 1
		}
	}
	return 0
}

// procRealUID returns the real uid of pid from /proc/<pid>/status.
func procRealUID(pid int) (int, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			if f := strings.Fields(rest); len(f) > 0 {
				return strconv.Atoi(f[0])
			}
		}
	}
	return 0, os.ErrNotExist
}
