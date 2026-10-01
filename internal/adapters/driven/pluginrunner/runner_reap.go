//go:build linux

package pluginrunner

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ReapStaleProcesses terminates plugin processes left behind by a previous
// server instance. PR_SET_PDEATHSIG is the first line of defense but is not
// airtight: the kernel clears it when execve'ing a binary with file caps
// (the prod launcher carries cap_setuid), and group-exit delivery has been
// observed to race on some kernels (WSL2) - each leaked node process holds
// ~25 MB. Call once at boot, BEFORE starting plugins.
//
// A process is reaped only when ALL of these hold:
//   - its cmdline references a path under one of the runner's plugin dirs
//     (the way every spawned plugin does: <dir>/<plugin-id>/main.js),
//   - it is not this process and not registered in r.processes,
//   - it runs as the server's uid or the dedicated plugin uid (never an
//     operator's unrelated process).
//
// Termination mirrors Stop: SIGTERM to the process group (plugins are
// spawned with Setpgid, so pgid == pid), then SIGKILL after killTimeout for
// survivors, without blocking boot.
func (r *Runner) ReapStaleProcesses() int {
	self := os.Getpid()
	myUID := os.Getuid()

	r.mu.RLock()
	owned := make(map[int]bool, len(r.processes))
	for _, e := range r.processes {
		if e.cmd != nil && e.cmd.Process != nil {
			owned[e.cmd.Process.Pid] = true
		}
	}
	r.mu.RUnlock()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		slog.Warn("reap stale plugins: read /proc", slog.Any("err", err))
		return 0
	}

	var victims []int
	for _, ent := range entries {
		pid, convErr := strconv.Atoi(ent.Name())
		if convErr != nil || pid == self || owned[pid] {
			continue
		}
		if !r.cmdlineUnderPluginDir(pid) {
			continue
		}
		uid, uidErr := procUID(pid)
		if uidErr != nil || (uid != myUID && uid != pluginUID) {
			continue
		}
		victims = append(victims, pid)
	}

	// An announced sweep must have happened: count what could not be
	// signalled and say it once, loudly, with the likely cause. A warning per
	// pid on every boot read as noise while the orphans piled up (audit
	// 2026-10-01: EPERM on every one, the container had no CAP_KILL).
	var denied []int
	for _, pid := range victims {
		slog.Warn("reaping stale plugin process from a previous server instance",
			slog.Int("pid", pid))
		if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
			if killErr := syscall.Kill(pid, syscall.SIGTERM); killErr != nil && killErr != syscall.ESRCH {
				if killErr == syscall.EPERM {
					denied = append(denied, pid)
					continue
				}
				slog.Warn("reap stale plugin: SIGTERM failed", slog.Int("pid", pid), slog.Any("err", killErr))
			}
		}
	}
	if len(denied) > 0 {
		slog.Error("reap stale plugins: not permitted to signal leftover plugin processes - they keep running; grant the container CAP_KILL (plugins run under their own uid)",
			slog.Int("count", len(denied)), slog.Any("pids", denied))
	}
	if len(victims) > 0 {
		go func(pids []int) {
			time.Sleep(killTimeout)
			for _, pid := range pids {
				// Liveness alone is NOT identity. Across the grace period the
				// victim usually dies, and Linux is free to hand its pid to any
				// new process - kill(pid,0) then succeeds for a stranger, and we
				// SIGKILL it. Re-run the same two checks that made it a victim,
				// so an escalation only ever lands on a process that is STILL a
				// plugin of ours.
				if !r.stillReapable(pid, myUID) {
					continue
				}
				if syscall.Kill(pid, 0) == nil {
					if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
						if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil && killErr != syscall.ESRCH {
							slog.Warn("reap stale plugin: SIGKILL failed", slog.Int("pid", pid), slog.Any("err", killErr))
						}
					}
				}
			}
		}(victims)
	}
	return len(victims)
}

// cmdlineUnderPluginDir reports whether any argv element of pid points inside
// one of the runner's plugin directories. Covers direct spawns (argv[0] =
// interpreter, argv[1] = <dir>/<id>/main.js) as well as launcher- and
// bwrap-wrapped ones (the plugin path appears later in the argv).
func (r *Runner) cmdlineUnderPluginDir(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil || len(data) == 0 {
		return false
	}
	for arg := range strings.SplitSeq(string(data), "\x00") {
		for _, dir := range r.pluginDirs {
			if dir != "" && dir != "/" && strings.HasPrefix(arg, dir+"/") {
				return true
			}
		}
	}
	return false
}

// stillReapable re-confirms that pid is the same kind of process the reap
// selected: a plugin under one of our dirs, owned by us. Used before the
// delayed SIGKILL, where a recycled pid would otherwise be killed blind.
func (r *Runner) stillReapable(pid, myUID int) bool {
	if !r.cmdlineUnderPluginDir(pid) {
		return false
	}
	uid, err := procUID(pid)
	return err == nil && (uid == myUID || uid == pluginUID)
}

// procUID returns the real uid of pid from /proc/<pid>/status.
func procUID(pid int) (int, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				return strconv.Atoi(fields[0])
			}
		}
	}
	return 0, os.ErrNotExist
}
