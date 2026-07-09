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

	for _, pid := range victims {
		slog.Warn("reaping stale plugin process from a previous server instance",
			slog.Int("pid", pid))
		if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	if len(victims) > 0 {
		go func(pids []int) {
			time.Sleep(killTimeout)
			for _, pid := range pids {
				if syscall.Kill(pid, 0) == nil {
					if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
						_ = syscall.Kill(pid, syscall.SIGKILL)
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
	for _, arg := range strings.Split(string(data), "\x00") {
		for _, dir := range r.pluginDirs {
			if dir != "" && dir != "/" && strings.HasPrefix(arg, dir+"/") {
				return true
			}
		}
	}
	return false
}

// procUID returns the real uid of pid from /proc/<pid>/status.
func procUID(pid int) (int, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				return strconv.Atoi(fields[0])
			}
		}
	}
	return 0, os.ErrNotExist
}
