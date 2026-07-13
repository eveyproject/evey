package loadtest

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// resources.go measures what a viewer actually COSTS the server, which is the
// only way a load test can say WHY it stopped rather than just when.
//
// Everything here is read from the server's own cgroup, not the host: ffmpeg
// encoders are children of the server process, so they live in the same cgroup
// and their CPU, memory and disk reads are already counted. The one number
// that does NOT come from the kernel is the served throughput - the viewers
// tally the bytes they actually pulled, which is exactly what the server had
// to push, independent of any NIC counter (the simulated viewers talk to the
// server over loopback).
//
// Honest limitation, stated once here rather than hidden: the simulated
// viewers run inside that same cgroup, so their own HTTP overhead is included
// in the CPU figure. A client that reads a response and throws it away is
// cheap next to an encoder, and the run records a pre-viewer baseline so an
// operator can see the floor, but the CPU cost per viewer is an upper bound,
// not a laboratory measurement.

// Saturation thresholds. These are what turn "it got slow" into "the CPU ran
// out": a resource this close to its ceiling has nothing left to give a new
// viewer, and the next one would stall.
const (
	// cpuSaturationPercent is the share of the server's CPU allowance
	// (cgroup quota when set, else every core) past which the machine has no
	// headroom left for another encode.
	cpuSaturationPercent = 92.0
	// memSaturationPercent is the share of the memory limit past which the
	// next allocation risks the OOM killer rather than a slowdown.
	memSaturationPercent = 90.0
	// saturationSamples is how many CONSECUTIVE samples must sit above a
	// threshold before it counts as saturation. A single spike (an encoder
	// starting, a GC pause) is not a capacity verdict.
	saturationSamples = 3
)

// resourceSampleEvery is the cadence at which the run samples CPU/memory/IO.
// Fast enough to see a tier's shape (~15 samples per 30s measurement window),
// slow enough that the sampling itself costs nothing measurable.
const resourceSampleEvery = 2 * time.Second

// resourceWarmup is how long after a viewer joins its tier's samples are
// DISCARDED. A joining viewer pays an ffmpeg cold start and then bursts
// segments to fill its buffer; sampling through that transient would measure
// the start-up, not the steady state. Everything after it is the measurement
// window.
const resourceWarmup = 30 * time.Second

// ResourceSample is one instantaneous reading of what the server is consuming.
type ResourceSample struct {
	CPUPercent          float64
	MemUsedBytes        int64
	MemLimitBytes       int64
	DiskReadBytesPerSec int64
	ServedBytesPerSec   int64
}

// ResourceStats is what a whole tier consumed: the average is what a viewer
// costs in steady state, the peak is what the machine had to absorb.
type ResourceStats struct {
	CPUPercentAvg, CPUPercentPeak    float64
	MemUsedBytesAvg, MemUsedBytesMax int64
	MemLimitBytes                    int64
	DiskReadBytesPerSecAvg           int64
	ServedBytesPerSecAvg             int64
	Samples                          int
}

// counterSource is where the raw monotonic counters come from. The server's
// own cgroup is the only implementation that ships; the seam exists so a test
// can model a machine's load curve directly instead of racing a goroutine
// against the kernel's.
type counterSource interface {
	// cpuUsageUsec is cumulative CPU time consumed, in microseconds.
	cpuUsageUsec() int64
	// memory is current usage and the ceiling it is measured against.
	memory() (used, limit int64)
	// diskReadBytes is cumulative bytes read from disk.
	diskReadBytes() int64
	// cpuAllowance is how many cores may actually be used - the denominator
	// of the CPU percentage.
	cpuAllowance() float64
}

// resourceSampler turns monotonic counters into per-second rates. It is
// stateful: every reading is a delta against the previous one, so the first
// sample after construction is discarded as a baseline.
type resourceSampler struct {
	src  counterSource
	ncpu float64

	lastAt        time.Time
	lastCPUUsec   int64
	lastDiskRead  int64
	lastServed    int64
	hasPrevSample bool
}

func newResourceSampler(src counterSource) *resourceSampler {
	return &resourceSampler{src: src, ncpu: src.cpuAllowance()}
}

// sample reads the current counters and returns the rates since the previous
// call. servedTotal is the running total of bytes the viewers have pulled.
// ok is false for the very first call (no previous reading to diff against).
func (s *resourceSampler) sample(servedTotal int64, now time.Time) (ResourceSample, bool) {
	cpuUsec := s.src.cpuUsageUsec()
	diskRead := s.src.diskReadBytes()
	memUsed, memLimit := s.src.memory()

	prevAt, prevCPU, prevDisk, prevServed := s.lastAt, s.lastCPUUsec, s.lastDiskRead, s.lastServed
	had := s.hasPrevSample

	s.lastAt, s.lastCPUUsec, s.lastDiskRead, s.lastServed = now, cpuUsec, diskRead, servedTotal
	s.hasPrevSample = true

	if !had {
		return ResourceSample{}, false
	}
	elapsed := now.Sub(prevAt).Seconds()
	if elapsed <= 0 {
		return ResourceSample{}, false
	}

	out := ResourceSample{
		MemUsedBytes:  memUsed,
		MemLimitBytes: memLimit,
	}
	if cpuUsec > 0 && prevCPU > 0 && s.ncpu > 0 {
		// usage is in microseconds of CPU time; the wall clock gives the
		// budget (elapsed x cores), so the ratio is the share of the machine.
		out.CPUPercent = float64(cpuUsec-prevCPU) / (elapsed * 1e6 * s.ncpu) * 100
	}
	if diskRead > 0 && prevDisk > 0 {
		out.DiskReadBytesPerSec = int64(float64(diskRead-prevDisk) / elapsed)
	}
	if servedTotal >= prevServed {
		out.ServedBytesPerSec = int64(float64(servedTotal-prevServed) / elapsed)
	}
	return out, true
}

// resourceState accumulates samples per tier, skipping each tier's warm-up,
// and tracks how close to saturation the machine currently is.
type resourceState struct {
	byTier map[int][]ResourceSample

	// tierStartedAt is when the current tier gained its viewer; samples before
	// tierStartedAt+warmup are start-up transients, not steady state.
	tierStartedAt time.Time
	warmup        time.Duration
	lastTier      int

	// baseline is what the server consumed BEFORE any viewer joined - the
	// floor an operator must subtract to read a per-viewer cost honestly.
	baseline ResourceSample

	cpuOverRun, memOverRun int
}

func newResourceState(warmup time.Duration, startedAt time.Time) *resourceState {
	return &resourceState{
		byTier:        map[int][]ResourceSample{},
		tierStartedAt: startedAt,
		warmup:        warmup,
	}
}

// observe records one sample against the current tier and reports whether a
// resource has been saturated for long enough to count as a verdict. Samples
// inside a tier's warm-up window are dropped, and never contribute to
// saturation: a tier is only ever judged once it has settled.
func (rs *resourceState) observe(tier int, s ResourceSample, now time.Time) (saturated StopReason, detail string) {
	if tier != rs.lastTier {
		// The ramp moved: this tier is in transition, warm up again.
		rs.lastTier = tier
		rs.tierStartedAt = now
		rs.cpuOverRun, rs.memOverRun = 0, 0
	}
	if tier == 0 {
		rs.baseline = s
		return "", ""
	}
	if now.Sub(rs.tierStartedAt) < rs.warmup {
		return "", ""
	}
	rs.byTier[tier] = append(rs.byTier[tier], s)

	if s.CPUPercent >= cpuSaturationPercent {
		rs.cpuOverRun++
	} else {
		rs.cpuOverRun = 0
	}
	if s.MemLimitBytes > 0 && float64(s.MemUsedBytes)/float64(s.MemLimitBytes)*100 >= memSaturationPercent {
		rs.memOverRun++
	} else {
		rs.memOverRun = 0
	}

	switch {
	case rs.cpuOverRun >= saturationSamples:
		return StopCPUSaturated, formatCPUDetail(s.CPUPercent, tier)
	case rs.memOverRun >= saturationSamples:
		return StopMemorySaturated, formatMemDetail(s.MemUsedBytes, s.MemLimitBytes, tier)
	}
	return "", ""
}

// stats folds a tier's samples into the averages and peaks the report carries.
func (rs *resourceState) stats(tier int) ResourceStats {
	samples := rs.byTier[tier]
	if len(samples) == 0 {
		return ResourceStats{}
	}
	var out ResourceStats
	var cpuSum, memSum, diskSum, servedSum float64
	for _, s := range samples {
		cpuSum += s.CPUPercent
		memSum += float64(s.MemUsedBytes)
		diskSum += float64(s.DiskReadBytesPerSec)
		servedSum += float64(s.ServedBytesPerSec)
		out.CPUPercentPeak = max(out.CPUPercentPeak, s.CPUPercent)
		out.MemUsedBytesMax = max(out.MemUsedBytesMax, s.MemUsedBytes)
		out.MemLimitBytes = max(out.MemLimitBytes, s.MemLimitBytes)
	}
	n := float64(len(samples))
	out.CPUPercentAvg = cpuSum / n
	out.MemUsedBytesAvg = int64(memSum / n)
	out.DiskReadBytesPerSecAvg = int64(diskSum / n)
	out.ServedBytesPerSecAvg = int64(servedSum / n)
	out.Samples = len(samples)
	return out
}

// perViewerCost is what ONE additional viewer costs, read as the slope between
// the first measured tier and the sustained one rather than "total / viewers":
// the baseline (an idle server, plus the harness itself) must not be charged to
// the viewers. Returns false when fewer than two tiers were measured, in which
// case there is no slope to read.
func (rs *resourceState) perViewerCost(sustainedTier int) (ResourceStats, bool) {
	lo, hi := 0, 0
	for tier := 1; tier <= sustainedTier; tier++ {
		if len(rs.byTier[tier]) == 0 {
			continue
		}
		if lo == 0 {
			lo = tier
		}
		hi = tier
	}
	if lo == 0 || hi <= lo {
		return ResourceStats{}, false
	}
	loS, hiS := rs.stats(lo), rs.stats(hi)
	span := float64(hi - lo)
	return ResourceStats{
		CPUPercentAvg:          (hiS.CPUPercentAvg - loS.CPUPercentAvg) / span,
		MemUsedBytesAvg:        int64(float64(hiS.MemUsedBytesAvg-loS.MemUsedBytesAvg) / span),
		DiskReadBytesPerSecAvg: int64(float64(hiS.DiskReadBytesPerSecAvg-loS.DiskReadBytesPerSecAvg) / span),
		ServedBytesPerSecAvg:   int64(float64(hiS.ServedBytesPerSecAvg-loS.ServedBytesPerSecAvg) / span),
		Samples:                hiS.Samples + loS.Samples,
	}, true
}

func formatCPUDetail(pct float64, tier int) string {
	return "CPU saturated at " + strconv.FormatFloat(pct, 'f', 0, 64) + "% with " + strconv.Itoa(tier) + " viewers"
}

func formatMemDetail(used, limit int64, tier int) string {
	pct := float64(used) / float64(limit) * 100
	return "memory saturated at " + strconv.FormatFloat(pct, 'f', 0, 64) + "% of the limit with " + strconv.Itoa(tier) + " viewers"
}

// --- kernel counters --------------------------------------------------------
//
// cgroup v2 first (the server runs in a container, and its cgroup already
// contains every ffmpeg it spawned), host /proc as a fallback for a bare-metal
// or cgroup-v1 install. A counter that can't be read reports 0, and the report
// simply omits that resource rather than inventing a number.

// cgroupSource reads the server's own cgroup. fsRoot is "/" in production;
// tests point it at a staged tree.
type cgroupSource struct{ fsRoot string }

func (c cgroupSource) cpuUsageUsec() int64         { return readCPUUsageUsec(c.fsRoot) }
func (c cgroupSource) diskReadBytes() int64        { return readDiskReadBytes(c.fsRoot) }
func (c cgroupSource) memory() (used, limit int64) { return readMemory(c.fsRoot) }
func (c cgroupSource) cpuAllowance() float64       { return cpuAllowance(c.fsRoot) }

// cpuAllowance is how many cores the server may actually use: the cgroup quota
// when one is set, else every core on the machine. This is the denominator of
// the CPU percentage, so a container capped at 2 cores on a 16-core host
// saturates at 2, exactly as the operator configured.
func cpuAllowance(fsRoot string) float64 {
	raw, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/cpu.max"))
	if err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) == 2 && fields[0] != "max" {
			quota, qErr := strconv.ParseFloat(fields[0], 64)
			period, pErr := strconv.ParseFloat(fields[1], 64)
			if qErr == nil && pErr == nil && quota > 0 && period > 0 {
				return quota / period
			}
		}
	}
	return float64(runtime.NumCPU())
}

// readCPUUsageUsec returns the cgroup's cumulative CPU time in microseconds.
func readCPUUsageUsec(fsRoot string) int64 {
	f, err := os.Open(filepath.Join(fsRoot, "sys/fs/cgroup/cpu.stat"))
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		field, value, ok := strings.Cut(scanner.Text(), " ")
		if ok && field == "usage_usec" {
			v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return 0
			}
			return v
		}
	}
	return 0
}

// readMemory returns the cgroup's current usage and its limit. An unlimited
// cgroup ("max") falls back to the machine's total RAM, which IS the real
// ceiling in that case.
func readMemory(fsRoot string) (used, limit int64) {
	if raw, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/memory.current")); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64); err == nil {
			used = v
		}
	}
	if raw, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/memory.max")); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64); err == nil && v > 0 {
			limit = v
		}
	}
	if used == 0 || limit == 0 {
		total, available := readMeminfo(fsRoot)
		if used == 0 && total > 0 {
			used = total - available
		}
		if limit == 0 {
			limit = total
		}
	}
	return used, limit
}

// readDiskReadBytes returns the cgroup's cumulative bytes read from disk,
// summed across every block device. This is the number that says whether the
// storage - not the CPU - is what a viewer is really costing.
func readDiskReadBytes(fsRoot string) int64 {
	f, err := os.Open(filepath.Join(fsRoot, "sys/fs/cgroup/io.stat"))
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	var total int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Each line is "<major>:<minor> rbytes=N wbytes=N rios=N ...".
		for field := range strings.FieldsSeq(scanner.Text()) {
			key, value, ok := strings.Cut(field, "=")
			if !ok || key != "rbytes" {
				continue
			}
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				total += v
			}
		}
	}
	return total
}

// readMeminfo is the bare-metal fallback for readMemory.
func readMeminfo(fsRoot string) (total, available int64) {
	f, err := os.Open(filepath.Join(fsRoot, "proc/meminfo"))
	if err != nil {
		return 0, 0
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = parseMeminfoKB(line)
		}
		if total > 0 && available > 0 {
			break
		}
	}
	return total, available
}

func parseMeminfoKB(line string) int64 {
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
