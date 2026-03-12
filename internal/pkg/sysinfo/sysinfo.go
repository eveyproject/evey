package sysinfo

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Stats struct {
	CPUPercent    float64
	MemoryPercent float64
	MemUsedBytes  int64
	MemTotalBytes int64
}

type StorageStats struct {
	Path       string
	TotalBytes int64
	UsedBytes  int64
	FreeBytes  int64
}

type cpuSample struct {
	idle  uint64
	total uint64
}

func Collect() Stats {
	var s Stats

	memTotal, memAvail := readMeminfo()
	if memTotal > 0 {
		s.MemTotalBytes = memTotal
		s.MemUsedBytes = memTotal - memAvail
		s.MemoryPercent = float64(s.MemUsedBytes) / float64(memTotal) * 100
	} else {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		s.MemUsedBytes = int64(m.Sys)
	}

	s.CPUPercent = sampleCPU()
	return s
}

func GetStorageStats(paths []string) []StorageStats {
	seen := make(map[string]bool)
	var result []StorageStats
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		var stat syscall.Statfs_t
		if err := syscall.Statfs(p, &stat); err != nil {
			continue
		}
		total := int64(stat.Blocks) * stat.Bsize
		free := int64(stat.Bavail) * stat.Bsize
		result = append(result, StorageStats{
			Path:       p,
			TotalBytes: total,
			UsedBytes:  total - free,
			FreeBytes:  free,
		})
	}
	return result
}

func readMeminfo() (total, available int64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available = parseKB(line)
		}
		if total > 0 && available > 0 {
			break
		}
	}
	return total, available
}

func parseKB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	kb, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return kb * 1024
}

func sampleCPU() float64 {
	s1, ok := readCPUStat()
	if !ok {
		return 0
	}
	time.Sleep(200 * time.Millisecond)
	s2, ok := readCPUStat()
	if !ok {
		return 0
	}
	dTotal := s2.total - s1.total
	dIdle := s2.idle - s1.idle
	if dTotal == 0 {
		return 0
	}
	return float64(dTotal-dIdle) / float64(dTotal) * 100
}

func readCPUStat() (cpuSample, bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return cpuSample{}, false
		}
		var total, idle uint64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
			if i == 4 { // idle is field 4
				idle = v
			}
		}
		return cpuSample{idle: idle, total: total}, true
	}
	return cpuSample{}, false
}
