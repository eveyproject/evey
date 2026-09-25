package sysinfo

import (
	"bufio"
	"context"
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
	CPUPercent float64
	// EveyCPUPercent is the share of the same host capacity taken by this
	// server and every process it forked, so an admin can tell Evey's load
	// from a neighbour's (Jellyfin on the same VM, a file copy).
	EveyCPUPercent float64
	MemoryPercent  float64
	MemUsedBytes   int64
	MemTotalBytes  int64
}

type StorageStats struct {
	Path       string
	Paths      []string
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

	s.CPUPercent, s.EveyCPUPercent = sampleCPU()

	cachedStats = s
	cacheExpiresAt = time.Now().Add(collectCacheTTL)
	return s
}

// GetStorageStats reports disk usage for the given paths, grouped by physical
// disk. Several libraries often live on the same drive (e.g. /mnt/disk1/movies
// and /mnt/disk1/tv); reporting each path separately would count the same
// disk's capacity once per library. We group by the filesystem device id
// (st_dev): paths that share a device are the same disk, so they collapse into
// a single entry whose Paths lists every library on that drive.
func GetStorageStats(paths []string) []StorageStats {
	byDevice := make(map[uint64]int) // device id -> index into result
	seenPath := make(map[string]bool)
	var result []StorageStats
	for _, p := range paths {
		if seenPath[p] {
			continue
		}
		seenPath[p] = true

		var st syscall.Stat_t
		if err := syscall.Stat(p, &st); err != nil {
			continue
		}
		if idx, ok := byDevice[st.Dev]; ok {
			// Same physical disk as an earlier library - record the path but
			// don't double-count the capacity.
			result[idx].Paths = append(result[idx].Paths, p)
			continue
		}

		var fs syscall.Statfs_t
		if err := syscall.Statfs(p, &fs); err != nil {
			continue
		}
		total := int64(fs.Blocks) * fs.Bsize
		free := int64(fs.Bavail) * fs.Bsize
		byDevice[st.Dev] = len(result)
		result = append(result, StorageStats{
			Path:       p,
			Paths:      []string{p},
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

// sampleCPU returns the host's busy share and the part of it this server's
// process tree accounts for, over the same window.
func sampleCPU() (host, evey float64) {
	self := os.Getpid()
	s1, ok := readCPUStat()
	if !ok {
		return 0, 0
	}
	t1 := treeCPUTicks(self)
	time.Sleep(500 * time.Millisecond)
	s2, ok := readCPUStat()
	if !ok {
		return 0, 0
	}
	t2 := treeCPUTicks(self)
	dTotal := s2.total - s1.total
	if dTotal == 0 {
		return 0, 0
	}
	host = float64(dTotal-(s2.idle-s1.idle)) / float64(dTotal) * 100
	if t2 > t1 {
		evey = min(host, float64(t2-t1)/float64(dTotal)*100)
	}
	return host, evey
}

// CPUBusyPercent samples /proc/stat twice, window apart, and returns the share
// of CPU time the whole host spent NOT idle (0-100). Everything on the box
// counts - other containers, the embedded Postgres and Meilisearch, a scanner,
// a neighbour VM's steal - which is the point: it answers "is this host free
// to be measured right now?", the question the encode calibration asks before
// trusting its own benchmark. 0 when /proc/stat cannot be read.
func CPUBusyPercent(window time.Duration) float64 {
	s1, ok := readCPUStat()
	if !ok {
		return 0
	}
	time.Sleep(window)
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
	ffmpegMu     sync.Mutex
	ffmpegCached string
	ffmpegProbed bool
	// ffmpegRetryAt dates the next attempt allowed after a FAILURE. Without it,
	// "only successes are remembered" means a host with no ffmpeg re-forks a
	// process on EVERY call while holding ffmpegMu for up to ffmpegProbeTimeout,
	// so a polling admin dashboard serialises 3s per refresh. The window keeps
	// the intent (a transient failure heals) without the cost.
	ffmpegRetryAt time.Time
	// ffmpegProbes counts REAL executions, so a test can tell "answered from
	// cache" from "forked again".
	ffmpegProbes int
	// ffmpegNow is swappable in tests to cross the window without waiting.
	ffmpegNow = time.Now
)

// ffmpegProbeTimeout bounds the version probe. `ffmpeg -version` answers in
// milliseconds; anything slower means the binary is not reachable in a useful
// way (a stalled network mount, a wrapper script waiting on something).
const ffmpegProbeTimeout = 3 * time.Second

// ffmpegFailureRetryAfter is how long a failed probe is remembered. Short
// enough that fixing the host (remount, install) shows up without a restart,
// long enough that a refreshing admin panel does not re-fork every round.
const ffmpegFailureRetryAfter = 30 * time.Second

// FFmpegVersion runs `ffmpeg -version` and caches the result.
//
// The probe is bounded and a FAILURE is not cached. It used to run under a
// sync.Once with no timeout: if that single execution ever hung - a stalled
// mount, a bad shim - the Once never completed, and every later caller from
// any goroutine blocked on it forever. One slow probe took out an admin
// endpoint permanently, until restart. A failed probe now simply retries next
// time; only a successful answer is remembered.
func FFmpegVersion(ffmpegPath string) string {
	ffmpegMu.Lock()
	defer ffmpegMu.Unlock()
	if ffmpegProbed {
		return ffmpegCached
	}
	if now := ffmpegNow(); now.Before(ffmpegRetryAt) {
		return ffmpegCached
	}
	func() {
		p := ffmpegPath
		if p == "" {
			p = "ffmpeg"
		}
		ffmpegProbes++
		// Window set BEFORE probing: on success ffmpegProbed takes over and the
		// window is moot; on failure or timeout it is already in place.
		ffmpegRetryAt = ffmpegNow().Add(ffmpegFailureRetryAfter)
		ctx, cancel := context.WithTimeout(context.Background(), ffmpegProbeTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, p, "-version").Output() //nolint:gosec
		if err != nil {
			return
		}
		line := strings.SplitN(string(out), "\n", 2)[0]
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "version" && i+1 < len(fields) {
				ffmpegCached = fields[i+1]
				ffmpegProbed = true
				return
			}
		}
	}()
	return ffmpegCached
}

// CPUSignature returns a stable hash of the host CPU identity (first
// `model name` line + total logical cores from /proc/cpuinfo). Designed
// to invalidate cached calibration data when a container migrates to a
// different host or the operator swaps the CPU. Identical hardware
// returns the same hash across restarts.
//
// Empty when /proc/cpuinfo is unreadable (non-Linux, locked-down
// sandbox) - callers should treat empty as "machine identity unknown"
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
		// user nice system idle iowait irq softirq steal [guest guest_nice]:
		// guest time is already inside user, and a core waiting on the disk
		// (iowait) is free to run anything - a file copy onto the library
		// used to read as CPU load.
		var total, idle uint64
		for i := 1; i < len(fields) && i <= 8; i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
			if i == 4 || i == 5 {
				idle += v
			}
		}
		return cpuSample{idle: idle, total: total}, true
	}
	return cpuSample{}, false
}
