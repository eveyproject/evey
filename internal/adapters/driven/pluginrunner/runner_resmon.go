package pluginrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	defaultMemBytes int64 = 256 * 1024 * 1024 // 256 MiB
	defaultPIDsMax  int   = 64
)

// resourceLimits are the per-plugin caps the monitor enforces. They are NOT
// cgroup limits (cgroups are unusable in a non-privileged Docker container);
// the sweeper polls /proc and acts in userspace.
type resourceLimits struct {
	memBytes int64
	pidsMax  int
}

// defaults fills unset (zero) fields with the package defaults.
func (l resourceLimits) defaults() resourceLimits {
	if l.memBytes == 0 {
		l.memBytes = defaultMemBytes
	}
	if l.pidsMax == 0 {
		l.pidsMax = defaultPIDsMax
	}
	return l
}

// dirSize returns the total size of regular files under dir. Unreadable entries
// are skipped so a transient permission/IO error never aborts the measurement.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info != nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// Resource monitoring without cgroups. Per-plugin cgroup limits are not
// achievable in a standard (non-privileged, cgroupns) Docker container, so a
// single background sweeper polls /proc for every running plugin and enforces
// caps in userspace: kill on memory / PID overrun, and flag sustained CPU
// pegging (a cryptominer signature). CPU starvation is prevented up-front by a
// low nice value set at spawn. Reading /proc is in-memory and cheap; the sweep
// is one pass over a handful of plugin PIDs every resMonInterval.
const (
	resMonInterval    = 2 * time.Second // sweep cadence
	resMonDiskEveryN  = 15              // run the (heavier) disk-size check every Nth sweep (~30s)
	resMonCPUPegPct   = 90              // sustained %CPU considered "pegging"
	resMonCPUPegTicks = 15              // consecutive pegging samples (~30s) before flagging
	clockTicksPerSec  = 100             // USER_HZ on Linux (sysconf(_SC_CLK_TCK))
)

// cpuSample is the per-plugin CPU accounting carried between sweeps.
type cpuSample struct {
	ticks   uint64    // utime+stime at last sample
	at      time.Time // wall clock at last sample
	pegRuns int       // consecutive samples above resMonCPUPegPct
}

// startResourceMonitor launches the sweeper once. It runs for the lifetime of
// the process; when no plugins are running each sweep is a no-op. The one-time
// log line lets ops confirm the monitor is armed and with which caps (the
// sweeper is otherwise silent until a plugin breaches a limit).
func (r *Runner) startResourceMonitor() {
	r.resMonOnce.Do(func() {
		lim, nice, disk := r.limitsSnapshot()
		cfg := lim.defaults()
		slog.Info("plugin resource monitor started",
			slog.Int64("mem_mb", cfg.memBytes>>20),
			slog.Int("pids_max", cfg.pidsMax),
			slog.Int64("disk_mb", disk>>20),
			slog.Int("cpu_nice", nice))
		go r.resourceMonitorLoop()
	})
}

func (r *Runner) resourceMonitorLoop() {
	ticker := time.NewTicker(resMonInterval)
	defer ticker.Stop()
	cpu := make(map[int]*cpuSample)
	sweep := 0
	for range ticker.C {
		sweep++
		r.mu.RLock()
		entries := make([]*processEntry, 0, len(r.processes))
		for _, e := range r.processes {
			entries = append(entries, e)
		}
		r.mu.RUnlock()

		live := make(map[int]bool, len(entries))
		for _, e := range entries {
			if e.cmd == nil || e.cmd.Process == nil {
				continue
			}
			pid := e.cmd.Process.Pid
			live[pid] = true
			r.checkResources(e, pid, cpu, sweep%resMonDiskEveryN == 0)
		}
		for pid := range cpu { // drop accounting for exited plugins
			if !live[pid] {
				delete(cpu, pid)
			}
		}
	}
}

// checkResources samples one plugin and enforces its caps.
func (r *Runner) checkResources(e *processEntry, pid int, cpu map[int]*cpuSample, checkDisk bool) {
	lim, _, diskMax := r.limitsSnapshot()
	cfg := lim.defaults()

	// Memory: kill if RSS exceeds the cap.
	if cfg.memBytes > 0 {
		if rss, err := procRSSBytes(pid); err == nil && rss > cfg.memBytes {
			r.killOverLimit(e, fmt.Sprintf("memory quota exceeded (%d MB > %d MB)", rss>>20, cfg.memBytes>>20))
			return
		}
	}

	// PID/thread count: kill if it exceeds the cap (fork-bomb guard, best-effort).
	if cfg.pidsMax > 0 {
		if n, err := procThreads(pid); err == nil && n > cfg.pidsMax {
			r.killOverLimit(e, fmt.Sprintf("thread/PID quota exceeded (%d > %d)", n, cfg.pidsMax))
			return
		}
	}

	// CPU: nice already prevents starvation; here we only DETECT sustained
	// pegging (cryptominer-like) and flag it once.
	if ticks, err := procCPUTicks(pid); err == nil {
		now := time.Now()
		prev, ok := cpu[pid]
		if !ok {
			cpu[pid] = &cpuSample{ticks: ticks, at: now}
		} else {
			dt := now.Sub(prev.at).Seconds()
			if dt > 0 {
				pct := float64(ticks-prev.ticks) / clockTicksPerSec / dt / float64(runtime.NumCPU()) * 100
				prev.ticks, prev.at = ticks, now
				if pct >= resMonCPUPegPct {
					prev.pegRuns++
					if prev.pegRuns == resMonCPUPegTicks {
						slog.Warn("plugin sustaining high CPU (possible resource abuse / miner)",
							slog.String("plugin", e.pluginID), slog.Float64("cpu_pct", pct))
					}
				} else {
					prev.pegRuns = 0
				}
			}
		}
	}

	// Disk: heavier (walks the dir) → only every Nth sweep.
	if checkDisk && diskMax > 0 && e.installDir != "" {
		if sz, err := dirSize(e.installDir); err == nil && sz > diskMax {
			r.killOverLimit(e, fmt.Sprintf("disk quota exceeded (%d MB > %d MB)", sz>>20, diskMax>>20))
		}
	}
}

func (r *Runner) killOverLimit(e *processEntry, reason string) {
	slog.Warn("stopping plugin: "+reason, slog.String("plugin", e.pluginID))
	r.publishPluginError(context.Background(), e.pluginID, reason+"; plugin stopped")
	_ = r.Stop(context.Background(), e.pluginID)
}

// applyNice lowers a freshly-started plugin's scheduling priority so it can
// never starve the server (and other higher-priority work) of CPU under
// contention. Lowering one's own child priority needs no privilege.
func (r *Runner) applyNice(pid int) {
	_, nice, _ := r.limitsSnapshot()
	if nice <= 0 {
		return
	}
	if err := unix.Setpriority(unix.PRIO_PROCESS, pid, nice); err != nil {
		slog.Debug("set plugin nice failed", slog.Int("pid", pid), slog.Any("err", err))
	}
}

// procRSSBytes returns the resident set size of pid from /proc/<pid>/statm
// (field 2 = resident pages). Cheap: a single in-memory read, O(1).
func procRSSBytes(pid int) (int64, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, fmt.Errorf("malformed statm")
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * int64(os.Getpagesize()), nil
}

// procThreads returns the thread count of pid from /proc/<pid>/status.
func procThreads(pid int) (int, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Threads:") {
			return strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Threads:")))
		}
	}
	return 0, fmt.Errorf("Threads not found")
}

// procCPUTicks returns utime+stime (clock ticks) of pid from /proc/<pid>/stat.
// Fields 14 (utime) and 15 (stime) are after the comm field, which can contain
// spaces/parens, so we split on the closing ')'.
func procCPUTicks(pid int) (uint64, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	s := string(data)
	rparen := strings.LastIndexByte(s, ')')
	if rparen < 0 {
		return 0, fmt.Errorf("malformed stat")
	}
	fields := strings.Fields(s[rparen+1:]) // fields[0] = state (field 3); utime=field14→idx 11, stime→idx 12
	if len(fields) < 13 {
		return 0, fmt.Errorf("short stat")
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, err
	}
	return utime + stime, nil
}
