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
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	s := string(data)
	rparen := strings.LastIndexByte(s, ')')
	if rparen < 0 {
		return 0, errors.New("malformed stat")
	}
	fields := strings.Fields(s[rparen+1:]) // fields[0] = state (field 3); utime = idx 11, stime = idx 12
	if len(fields) < 13 {
		return 0, errors.New("short stat")
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(utime+stime) / clockTicksPerSecond, nil
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
