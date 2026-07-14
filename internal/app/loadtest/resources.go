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

// ResourceSample is one instantaneous reading of what the server is consuming.
type ResourceSample struct {
	CPUPercent          float64
	MemUsedBytes        int64
	MemLimitBytes       int64
	DiskReadBytesPerSec int64
	// AppReadBytesPerSec is what the server ASKED the filesystem for, as opposed
	// to what the disk delivered (DiskReadBytesPerSec). The difference is the
	// kernel's readahead.
	AppReadBytesPerSec int64
	ServedBytesPerSec  int64
}

// ResourceStats is what a whole tier consumed: the average is what a viewer
// costs in steady state, the peak is what the machine had to absorb.
type ResourceStats struct {
	CPUPercentAvg, CPUPercentPeak    float64
	MemUsedBytesAvg, MemUsedBytesMax int64
	MemLimitBytes                    int64
	DiskReadBytesPerSecAvg           int64
	AppReadBytesPerSecAvg            int64
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
	// diskReadBytes is cumulative bytes the BLOCK LAYER actually fetched - what
	// the hardware really did.
	diskReadBytes() int64
	// appReadBytes is cumulative bytes the server ASKED FOR through read()
	// syscalls - what the code actually wants, page cache or not.
	//
	// The two are not the same number, and the gap is the whole point. The kernel
	// reads AHEAD: it fetches 128 KiB windows whether or not anyone wanted them.
	// On sequential reads that is free (the next read is already in memory); on
	// scattered ones - twenty viewers, twenty films, twenty offsets - it is waste
	// that no application-level measurement can see. Reporting only the block
	// layer says "the server reads 2.5x what it serves" without saying whether
	// that is the CODE being greedy or the KERNEL guessing wrong - and those two
	// have entirely different fixes.
	appReadBytes() int64
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
	lastAppRead   int64
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
	appRead := s.src.appReadBytes()
	memUsed, memLimit := s.src.memory()

	prevAt, prevCPU, prevDisk, prevServed := s.lastAt, s.lastCPUUsec, s.lastDiskRead, s.lastServed
	prevApp := s.lastAppRead
	had := s.hasPrevSample

	s.lastAt, s.lastCPUUsec, s.lastDiskRead, s.lastServed = now, cpuUsec, diskRead, servedTotal
	s.lastAppRead = appRead
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
	if appRead > 0 && prevApp > 0 {
		out.AppReadBytesPerSec = int64(float64(appRead-prevApp) / elapsed)
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

// settling reports that the tier is still in its warm-up window: viewers that
// just joined are burst-filling their buffers, and the latency they see (and
// impose) is a start-up transient, not what the tier costs in steady state.
// Judging a tier on those samples condemns tiers that would have held.
func (rs *resourceState) settling(now time.Time) bool {
	return now.Sub(rs.tierStartedAt) < rs.warmup
}

// startTier begins a new tier's warm-up RIGHT NOW, rather than whenever the
// next resource sample happens to land.
//
// The sampler ticks every couple of seconds; leaving it to notice the change
// means the tier's first seconds are judged with no warm-up at all - and those
// are exactly the seconds when requests issued under the PREVIOUS (overloaded)
// tier are still landing. A tier stepped back down from an overload would
// inherit its slow fetches and be condemned for a stall it never had.
func (rs *resourceState) startTier(tier int, now time.Time) {
	rs.lastTier = tier
	rs.tierStartedAt = now
	rs.cpuOverRun, rs.memOverRun = 0, 0
}

// setWarmup changes how much of a tier is discarded as transient. The climb
// hunts on a short hold (where a long warm-up would leave no samples at all)
// and judges on a long one, so the warm-up follows the phase.
func (rs *resourceState) setWarmup(d time.Duration) { rs.warmup = d }

// throughputGainThreshold is how much more data a tier must push, relative to
// the best any smaller tier managed, for the extra viewers to count as actually
// being served. Below it, the server is handing out the same bytes to more
// people - which is what a saturated SOURCE looks like.
const throughputGainThreshold = 1.05

// minPlateauBytesPerSec is the floor below which a flat throughput curve proves
// nothing: a server dribbling a few hundred KB/s has not hit its storage
// ceiling, it is being held back by something else entirely.
const minPlateauBytesPerSec = 8 << 20 // 8 MiB/s

// throughputPlateaued reports that adding viewers has stopped adding
// throughput: the box is already pushing as many bytes per second as it can
// pull, and every new viewer just takes a slice of the same pie.
//
// This is the bottleneck no CPU or memory gauge can see. A media server's real
// ceiling is usually neither: it is how fast it can READ the films. When the CPU
// sits at 12%, memory at a third, and the bytes-out curve goes flat, the answer
// is the storage path - the disk, the array, or the protocol in front of it -
// and the run must say so rather than climb until something else breaks.
//
// It deliberately does not care WHICH storage: a network mount, a spinning disk
// or an over-subscribed SSD all present the same way, and the per-second figure
// the report carries is the number an operator can act on.
func (rs *resourceState) throughputPlateaued(tier int) (bool, int64) {
	cur := rs.stats(tier)
	if cur.Samples == 0 || cur.ServedBytesPerSecAvg <= 0 {
		return false, 0
	}
	var best int64
	for t, samples := range rs.byTier {
		if t >= tier || len(samples) == 0 {
			continue
		}
		best = max(best, rs.stats(t).ServedBytesPerSecAvg)
	}
	// A plateau only means something once real data is moving. Below this, the
	// run is measuring noise - and calling a trickle "the storage ceiling" would
	// blame the disk for whatever else is actually wrong.
	if best < minPlateauBytesPerSec {
		return false, 0
	}
	if float64(cur.ServedBytesPerSecAvg) < float64(best)*throughputGainThreshold {
		return true, best
	}
	return false, 0
}

// tierLoad is how hard the server was working at a tier: the most-loaded
// resource, as a fraction of its own ceiling (0..1). It is what tells the climb
// whether it can afford a big stride or should tread carefully - a box at 5% of
// its CPU is nowhere near its limit, and discovering that ten viewers at a time
// would waste everyone's evening.
func (rs *resourceState) tierLoad(tier int) float64 {
	s := rs.stats(tier)
	if s.Samples == 0 {
		return 0
	}
	load := s.CPUPercentAvg / 100
	if s.MemLimitBytes > 0 {
		load = max(load, float64(s.MemUsedBytesAvg)/float64(s.MemLimitBytes))
	}
	return min(load, 1)
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
	var cpuSum, memSum, diskSum, appSum, servedSum float64
	for _, s := range samples {
		cpuSum += s.CPUPercent
		memSum += float64(s.MemUsedBytes)
		diskSum += float64(s.DiskReadBytesPerSec)
		appSum += float64(s.AppReadBytesPerSec)
		servedSum += float64(s.ServedBytesPerSec)
		out.CPUPercentPeak = max(out.CPUPercentPeak, s.CPUPercent)
		out.MemUsedBytesMax = max(out.MemUsedBytesMax, s.MemUsedBytes)
		out.MemLimitBytes = max(out.MemLimitBytes, s.MemLimitBytes)
	}
	n := float64(len(samples))
	out.CPUPercentAvg = cpuSum / n
	out.MemUsedBytesAvg = int64(memSum / n)
	out.DiskReadBytesPerSecAvg = int64(diskSum / n)
	out.AppReadBytesPerSecAvg = int64(appSum / n)
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
func (c cgroupSource) appReadBytes() int64         { return readAppReadBytes(c.fsRoot) }
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

// readAppReadBytes returns the bytes the server has asked the filesystem for
// through read() syscalls (/proc/self/io: rchar), page cache included.
//
// This is the number the CODE is responsible for. Set against the block layer's
// (readDiskReadBytes), it separates two very different problems: a packager that
// reads more than it serves, and a kernel that reads ahead into data nobody
// wanted. It also works where the block counter cannot - a library on a network
// share or a 9p mount, where io.stat reports a confident zero.
func readAppReadBytes(fsRoot string) int64 {
	f, err := os.Open(filepath.Join(fsRoot, "proc/self/io"))
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		field, value, ok := strings.Cut(scanner.Text(), ": ")
		if ok && field == "rchar" {
			v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return 0
			}
			return v
		}
	}
	return 0
}
