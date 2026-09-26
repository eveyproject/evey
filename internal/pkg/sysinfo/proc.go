package sysinfo

import (
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// clockTicksPerSecond is the kernel's USER_HZ, the unit of the CPU time
// counters in /proc/<pid>/stat. Fixed at 100 on every Linux ABI Go targets;
// there is no cgo-free sysconf to ask, and nothing here runs elsewhere.
const clockTicksPerSecond = 100

// ProcRSSBytes returns the resident set size of pid from /proc/<pid>/statm.
func ProcRSSBytes(pid int) (int64, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, errors.New("malformed statm")
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * int64(os.Getpagesize()), nil
}

// ProcCPUSeconds returns the CPU time (user + system) pid has consumed so
// far, from /proc/<pid>/stat. Fields 14 (utime) and 15 (stime) sit after the
// comm field, which may hold spaces and parentheses, hence the split on the
// last ')'.
func ProcCPUSeconds(pid int) (float64, error) {
	_, ticks, err := procStat(pid)
	if err != nil {
		return 0, err
	}
	return float64(ticks) / clockTicksPerSecond, nil
}

// procStat reads the parent pid and the utime+stime ticks of pid.
func procStat(pid int) (ppid int, ticks uint64, err error) {
	ppid, ticks, _, err = procStatStart(pid)
	return ppid, ticks, err
}

// procStatStart is procStat plus the process start time (field 22, in ticks
// since boot), which tells a reused pid from the process that held it before.
func procStatStart(pid int) (ppid int, ticks, start uint64, err error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, 0, err
	}
	s := string(data)
	rparen := strings.LastIndexByte(s, ')')
	if rparen < 0 {
		return 0, 0, 0, errors.New("malformed stat")
	}
	fields := strings.Fields(s[rparen+1:]) // fields[0] = state (field 3); ppid = idx 1, utime = idx 11, stime = idx 12, starttime = idx 19
	if len(fields) < 20 {
		return 0, 0, 0, errors.New("short stat")
	}
	ppid, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, 0, err
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	start, err = strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	return ppid, utime + stime, start, nil
}

// proc names one process across samples: a pid alone can be reused.
type proc struct {
	pid   int
	start uint64
}

// treeCPUTicks sums the CPU ticks of root and every live descendant.
func treeCPUTicks(root int) uint64 {
	var sum uint64
	for _, t := range treeTicks([]int{root}) {
		sum += t
	}
	return sum
}

// treeTicks maps every live process under roots, the roots included, to its
// CPU ticks: the server's own goroutines plus the ffmpeg/ffprobe and plugins
// it forked, and the trees of any companion root.
func treeTicks(roots []int) map[proc]uint64 {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	parent := make(map[int]int, len(entries))
	seen := make(map[int]proc, len(entries))
	ticks := make(map[int]uint64, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		pp, t, start, err := procStatStart(pid)
		if err != nil {
			continue
		}
		parent[pid], ticks[pid], seen[pid] = pp, t, proc{pid, start}
	}
	isRoot := make(map[int]bool, len(roots))
	for _, r := range roots {
		isRoot[r] = true
	}
	out := make(map[proc]uint64)
	for pid, t := range ticks {
		for p := pid; p > 0; p = parent[p] {
			if isRoot[p] {
				out[seen[pid]] = t
				break
			}
			if parent[p] == p {
				break
			}
		}
	}
	return out
}

// windowTicks is the CPU the processes spent between two treeTicks samples,
// process by process. One born in between (a reused pid included, told apart
// by its start time) counts from zero. One that exited is lost for the window:
// comparing two sums instead took its whole life away, and a transcode ending
// mid-sample read as Evey at 0 % (measured: 198 ticks, then 0).
func windowTicks(before, after map[proc]uint64) uint64 {
	var sum uint64
	for p, t := range after {
		if b, ok := before[p]; ok && t >= b {
			sum += t - b
		} else if !ok {
			sum += t
		}
	}
	return sum
}

// SelfRSSBytes is ProcRSSBytes for this process.
func SelfRSSBytes() int64 {
	rss, err := ProcRSSBytes(os.Getpid())
	if err != nil {
		return 0
	}
	return rss
}

// LoadAverage returns the 1, 5 and 15 minute load averages from /proc/loadavg
// and false when it cannot be read (not Linux, /proc not mounted).
func LoadAverage() (l1, l5, l15 float64, ok bool) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, false
	}
	vals := make([]float64, 3)
	for i := range 3 {
		v, perr := strconv.ParseFloat(fields[i], 64)
		if perr != nil {
			return 0, 0, 0, false
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], true
}

// MemInfo is the host memory picture the health page shows: what the kernel
// reports as total and reclaimable-free, plus the cgroup budget when the
// container is capped below the host.
type MemInfo struct {
	TotalBytes     int64
	AvailableBytes int64
	// BudgetBytes is the cgroup memory limit when one applies, else 0.
	BudgetBytes int64
}

// ReadMemInfo collects MemInfo. On a host without /proc/meminfo the Go
// runtime's own view stands in for total (Sys), so the page never shows zero.
func ReadMemInfo() MemInfo {
	total, avail := readMeminfo()
	mi := MemInfo{TotalBytes: total, AvailableBytes: avail}
	if total == 0 {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		mi.TotalBytes = int64(m.Sys)
	}
	if b, ok := MemoryBudgetBytes(); ok && b > 0 && b < mi.TotalBytes {
		mi.BudgetBytes = b
	}
	return mi
}
