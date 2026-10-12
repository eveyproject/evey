package pluginrunner

import (
	"context"
	"fmt"
	"github.com/eveyproject/evey/internal/pkg/bgloop"
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
			return nil //nolint:nilerr // unreadable entries are skipped by design
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
	resMonGPUPegPct   = 90              // sustained % of a DRM engine considered "pegging" (miner)
	resMonGPUPegTicks = 15              // consecutive GPU-pegging samples (~30s) before KILLING
)

// cpuSample is the per-plugin CPU (and, for hardware.gpu plugins, GPU-engine)
// accounting carried between sweeps.
type cpuSample struct {
	ticks   uint64    // utime+stime at last sample
	at      time.Time // wall clock at last sample
	pegRuns int       // consecutive samples above resMonCPUPegPct

	// GPU DRM-engine accounting - only populated for plugins holding
	// hardware.gpu that actually opened the render node (have drm-engine fdinfo).
	gpuNanos   uint64    // summed drm-engine-* cumulative nanoseconds at last sample
	gpuAt      time.Time // wall clock at last GPU sample (zero = not yet sampled)
	gpuPegRuns int       // consecutive samples above resMonGPUPegPct
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
		// One plugin with an unreadable /proc entry must not take the whole
		// server down: this loop runs for the life of the process and nobody
		// waits on it.
		bgloop.Tick("pluginrunner.resourceMonitor", func() {
			sweep++
			// defer: under bgloop.Tick a panic is recovered, so a plain RUnlock
			// would not be reached and no plugin could register or deregister.
			entries := func() []*processEntry {
				r.mu.RLock()
				defer r.mu.RUnlock()
				out := make([]*processEntry, 0, len(r.processes))
				for _, e := range r.processes {
					out = append(out, e)
				}
				return out
			}()

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
		})
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

	// GPU: for plugins granted hardware.gpu, DETECT and KILL sustained DRM-engine
	// pegging - the cryptominer signature. The render node is shared with Evey's
	// own transcoder (Linux has no per-process GPU hard cap for VAAPI), so a
	// userspace kill-on-sustained-peg is the only enforcement layer. This is a
	// deterrent, NOT containment (a short kernel launch can still leak/exploit
	// before the ~30s window). fdinfo presence also gates it: a plugin that never
	// touched the GPU has no drm-engine counter and is skipped; the server's own
	// ffmpeg is never a plugin process, so it is never sampled here.
	if e.hasGPU {
		if nanos, ok := gpuEngineNanos(pid); ok {
			now := time.Now()
			prev := cpu[pid]
			switch {
			case prev == nil:
				cpu[pid] = &cpuSample{gpuNanos: nanos, gpuAt: now}
			case prev.gpuAt.IsZero():
				prev.gpuNanos, prev.gpuAt = nanos, now
			default:
				if dt := now.Sub(prev.gpuAt).Seconds(); dt > 0 {
					pct := float64(nanos-prev.gpuNanos) / 1e9 / dt * 100 // % of one engine-second
					prev.gpuNanos, prev.gpuAt = nanos, now
					if pct >= resMonGPUPegPct {
						prev.gpuPegRuns++
						if prev.gpuPegRuns >= resMonGPUPegTicks {
							r.killOverLimit(e, fmt.Sprintf("GPU engine pegged ~%.0f%% sustained (possible miner)", pct))
							return
						}
					} else {
						prev.gpuPegRuns = 0
					}
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
	_ = r.Stop(context.Background(), e.pluginID) //nolint:errcheck // best-effort stop of an over-limit plugin
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
	for line := range strings.SplitSeq(string(data), "\n") {
		if after, ok := strings.CutPrefix(line, "Threads:"); ok {
			return strconv.Atoi(strings.TrimSpace(after))
		}
	}
	return 0, fmt.Errorf("threads not found in /proc status")
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

// gpuEngineNanos sums the cumulative drm-engine-* nanosecond counters across the
// process's open DRM fds (/proc/<pid>/fdinfo/*). Returns (total, true) only when
// at least one drm-engine counter is present - i.e. the process actually holds a
// GPU render fd and is doing work. amdgpu (5.14+), i915/xe (5.19+) and recent
// nvidia expose these in fdinfo; dividing the delta by wall-clock gives the
// engine occupancy. Pure file reads, no privilege.
func gpuEngineNanos(pid int) (uint64, bool) {
	dir := fmt.Sprintf("/proc/%d/fdinfo", pid)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false
	}
	var total uint64
	found := false
	for _, ent := range entries {
		data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			continue // fd closed between ReadDir and ReadFile - fine
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			// e.g. "drm-engine-gfx:    123456789 ns"
			if !strings.HasPrefix(line, "drm-engine-") {
				continue
			}
			_, after, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			fields := strings.Fields(after)
			if len(fields) == 0 {
				continue
			}
			if n, perr := strconv.ParseUint(fields[0], 10, 64); perr == nil {
				total += n
				found = true
			}
		}
	}
	return total, found
}
