package sysinfo

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const collectCacheTTL = 30 * time.Second

var (
	collectMu      sync.Mutex
	cachedStats    Stats
	cacheExpiresAt time.Time
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
	collectMu.Lock()
	defer collectMu.Unlock()

	if time.Now().Before(cacheExpiresAt) {
		return cachedStats
	}

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

	cachedStats = s
	cacheExpiresAt = time.Now().Add(collectCacheTTL)
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

var (
	ffmpegOnce   sync.Once
	ffmpegCached string
)

// FFmpegVersion runs `ffmpeg -version` once and caches the result.
func FFmpegVersion(ffmpegPath string) string {
	ffmpegOnce.Do(func() {
		p := ffmpegPath
		if p == "" {
			p = "ffmpeg"
		}
		out, err := exec.Command(p, "-version").Output() //nolint:gosec
		if err != nil {
			return
		}
		line := strings.SplitN(string(out), "\n", 2)[0]
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "version" && i+1 < len(fields) {
				ffmpegCached = fields[i+1]
				return
			}
		}
	})
	return ffmpegCached
}

// CPUSignature returns a stable hash of the host CPU identity (first
// `model name` line + total logical cores from /proc/cpuinfo). Designed
// to invalidate cached calibration data when a container migrates to a
// different host or the operator swaps the CPU. Identical hardware
// returns the same hash across restarts.
//
// Empty when /proc/cpuinfo is unreadable (non-Linux, locked-down
// sandbox) — callers should treat empty as "machine identity unknown"
// and skip the auto-invalidation logic accordingly.
func CPUSignature() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()

	var model string
	var cores int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if model == "" && strings.HasPrefix(line, "model name") {
			if _, after, ok := strings.Cut(line, ":"); ok {
				model = strings.TrimSpace(after)
			}
		}
		if strings.HasPrefix(line, "processor") {
			cores++
		}
	}
	if model == "" && cores == 0 {
		return ""
	}
	h := sha256.Sum256([]byte(model + "|cores=" + strconv.Itoa(cores)))
	return hex.EncodeToString(h[:16]) // 16 bytes = 32 hex chars, enough for collision-free identity
}

// IsWSL reports whether the process runs under the Windows Subsystem for Linux.
// Used to flag HW backends whose driver path is broken there: WSL exposes the
// GPU through Mesa's experimental d3d12 VA driver, which can't sustain Evey's
// multi-variant VAAPI/QSV encode (it segfaults), so the admin UI greys those
// backends. NVENC (CUDA passthrough) is unaffected. Detected from the kernel
// version string, which carries "microsoft"/"WSL" on every WSL kernel.
func IsWSL() bool {
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(b))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
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
