package sysinfo

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// MemoryBudgetBytes returns the memory budget actually enforceable on this
// process: the container cgroup limit when one is set (v2 `memory.max`, then
// v1 `memory.limit_in_bytes`), else the host's MemTotal. ok is false when
// nothing readable exists (non-Linux, locked-down /proc).
//
// This is what "how much RAM do we have" must mean on a small NAS: the
// docker-compose limit, not the machine total. Sizing decisions (GOMEMLIMIT,
// job concurrency, the low-memory profile) key off this value.
func MemoryBudgetBytes() (int64, bool) {
	return memoryBudgetBytes("/")
}

// memoryBudgetBytes is MemoryBudgetBytes with a chrootable filesystem root,
// so tests can stage fake cgroup/meminfo trees.
func memoryBudgetBytes(fsRoot string) (int64, bool) {
	hostTotal, _ := readMeminfoTotal(filepath.Join(fsRoot, "proc/meminfo"))

	limit, ok := readCgroupMemLimit(fsRoot)
	if ok {
		// cgroup v1 encodes "no limit" as a huge sentinel; a limit above the
		// host's RAM is not enforceable either way, so clamp to the host.
		if hostTotal > 0 && limit > hostTotal {
			limit = hostTotal
		}
		return limit, true
	}
	if hostTotal > 0 {
		return hostTotal, true
	}
	return 0, false
}

func readCgroupMemLimit(fsRoot string) (int64, bool) {
	// cgroup v2
	if data, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/memory.max")); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "max" {
			if n, perr := strconv.ParseInt(s, 10, 64); perr == nil && n > 0 {
				return n, true
			}
		}
		return 0, false // v2 present but unlimited -> caller falls back to host total
	}
	// cgroup v1
	if data, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/memory/memory.limit_in_bytes")); err == nil {
		if n, perr := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); perr == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

func readMeminfoTotal(path string) (int64, bool) {
	return readMeminfoField(path, "MemTotal:")
}

// CPUBudget returns the number of WORKERS this process may run: the cgroup CPU
// quota when one is set (v2 `cpu.max`, then v1 cfs_quota/period), else
// runtime.NumCPU(). Fractional quotas round up (a 1.5-CPU limit can still run
// 2 workers at reduced speed). Never returns less than 1. For how much CPU
// time is actually available - the denominator of a capacity fraction - use
// CPUCapacity: a route burning 0.75 core is half of a 1.5-CPU container, not
// 37.5 % of the 2 workers this rounds it to.
func CPUBudget() int {
	return cpuBudget("/")
}

func cpuBudget(fsRoot string) int {
	if q, ok := readCgroupCPUQuota(fsRoot); ok {
		return ceilCores(q)
	}
	return runtime.NumCPU()
}

// CPUCapacity returns the CPU time this process may actually consume, in
// cores, exactly: quota/period of the cgroup limit when one is set (1.5 for
// `cpus: 1.5`), else runtime.NumCPU(). Never returns less than a hundredth
// of a core. This is the physical capacity admission budgets against.
func CPUCapacity() float64 {
	return cpuCapacity("/")
}

func cpuCapacity(fsRoot string) float64 {
	if q, ok := readCgroupCPUQuota(fsRoot); ok {
		return math.Max(q, 0.01)
	}
	return float64(runtime.NumCPU())
}

// readCgroupCPUQuota returns the exact quota in cores (quota/period).
func readCgroupCPUQuota(fsRoot string) (float64, bool) {
	// cgroup v2: "max 100000" or "<quota> <period>"
	if data, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/cpu.max")); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) == 2 && fields[0] != "max" {
			quota, qerr := strconv.ParseInt(fields[0], 10, 64)
			period, perr := strconv.ParseInt(fields[1], 10, 64)
			if qerr == nil && perr == nil && quota > 0 && period > 0 {
				return float64(quota) / float64(period), true
			}
		}
		return 0, false
	}
	// cgroup v1: quota -1 = unlimited
	quotaData, qerr := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/cpu/cpu.cfs_quota_us"))
	periodData, perr := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/cpu/cpu.cfs_period_us"))
	if qerr == nil && perr == nil {
		quota, qe := strconv.ParseInt(strings.TrimSpace(string(quotaData)), 10, 64)
		period, pe := strconv.ParseInt(strings.TrimSpace(string(periodData)), 10, 64)
		if qe == nil && pe == nil && quota > 0 && period > 0 {
			return float64(quota) / float64(period), true
		}
	}
	return 0, false
}

// ceilCores rounds a fractional quota up to whole workers, never under 1.
func ceilCores(cores float64) int {
	n := int(math.Ceil(cores))
	if n < 1 {
		return 1
	}
	return n
}

// MemAvailableBytes returns how much memory a new process may still claim
// without pushing the host or the container into reclaim: the kernel's
// MemAvailable, further bounded by the cgroup headroom (memory.max minus
// memory.current) when a limit is set. ok is false when nothing is readable.
func MemAvailableBytes() (int64, bool) {
	return memAvailableBytes("/")
}

func memAvailableBytes(fsRoot string) (int64, bool) {
	avail, ok := readMeminfoField(filepath.Join(fsRoot, "proc/meminfo"), "MemAvailable:")
	limit, limited := readCgroupMemLimit(fsRoot)
	if limited {
		if current, cok := readInt64File(filepath.Join(fsRoot, "sys/fs/cgroup/memory.current")); cok {
			headroom := max(limit-current, 0)
			if !ok || headroom < avail {
				return headroom, true
			}
		}
	}
	return avail, ok
}

func readMeminfoField(path, field string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, field); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				if kb, perr := strconv.ParseInt(fields[0], 10, 64); perr == nil {
					return kb * 1024, true
				}
			}
		}
	}
	return 0, false
}

func readInt64File(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	n, perr := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return n, perr == nil
}
