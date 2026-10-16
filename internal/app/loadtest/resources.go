package loadtest

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
// The simulated viewers run inside that same cgroup, so their own HTTP overhead
// lands in the same CPU counter as the server's work. That is not a footnote to
// be confessed and moved past: it is the difference between "a viewer costs 1.2%
// of CPU" and "a viewer costs 0.2%", and an operator sizing a NAS on the first
// figure buys six times the machine he needs. The run therefore MEASURES the
// harness rather than assuming it is negligible - see harnesscpu.go.

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
	// memCriticalPercent is the share of the memory limit past which the run
	// stops at once, on a single sample and inside a warm-up window. A join
	// burst can fill the machine in the seconds a normal verdict spends
	// warming up and confirming, and the OOM killer then takes the server.
	// Above this there is nothing left to measure.
	memCriticalPercent = 95.0
	// saturationSamples is how many CONSECUTIVE samples must sit above a
	// threshold before it counts as saturation. A single spike (an encoder
	// starting, a GC pause) is not a capacity verdict.
	saturationSamples = 3
)

// resourceSampleEvery is the cadence at which the run samples CPU/memory/IO.
// One second: a disk's burst (a join, a seek storm) lasts about that long, and
// a two-second average halved every peak the 2026-10-15 cross-check saw (sda
// at 41 % on the report, 59 % on a one-second probe). Reading a handful of
// /proc and cgroup files once a second costs nothing measurable.
const resourceSampleEvery = 1 * time.Second

// coldCacheEvictEvery is how often cold-cache mode re-drops the library from
// the page cache. Kept apart from the sampling cadence: an eviction makes the
// packager re-read through the mount (about a fifth more reads at two seconds,
// measured 2026-10-15), and sampling faster must not evict faster.
const coldCacheEvictEvery = 2 * time.Second

// baselineFor is how long the run watches the idle server before the first
// viewer joins. Every "saturated" verdict is read against this floor - above
// all the disks' wait at rest, which an SSD puts under a millisecond and a
// spinning pool at several - and a baseline taken from a single reading is a
// coin toss.
const baselineFor = 5 * time.Second

// ResourceSample is one instantaneous reading of what the server is consuming.
type ResourceSample struct {
	CPUPercent float64
	// MachineCPUPercent / MachineIOWaitPercent are the whole machine's share of
	// busy time and of time waiting on I/O (/proc/stat), beside the container's
	// figure above. The gap covers what serves the viewers from outside the
	// container (a FUSE pool, a VM host); iowait is the storage path saturating.
	MachineCPUPercent    float64
	MachineIOWaitPercent float64
	MemUsedBytes         int64
	// ProcessRSSBytes is the server process's own resident set, apart from the
	// container total (which also holds whatever else the image runs).
	ProcessRSSBytes int64
	// MemCacheBytes is the page cache the run accumulated by reading films. It
	// is NOT consumption: the kernel hands it straight back under pressure. It is
	// reported so an operator can see why "memory used" on a naive gauge climbs
	// to fill the box during a load test, and ignore it.
	MemCacheBytes int64
	MemLimitBytes int64
	// DiskReadBytesPerSec is what the block devices the library sits on read,
	// summed, machine-wide: every reader of those disks counts, not only this
	// server. DiskReadMeasured is false when no device could be tied to the
	// library (a network share, a mount this kernel cannot see through): the
	// figure is then absent, never a zero.
	DiskReadBytesPerSec int64
	DiskReadMeasured    bool
	// AppReadBytesPerSec is what the server ASKED the filesystem for, as opposed
	// to what the disk delivered (DiskReadBytesPerSec). The difference is the
	// kernel's readahead. AppReadMeasured is false when nothing counts those
	// reads in-process.
	AppReadBytesPerSec int64
	AppReadMeasured    bool
	ServedBytesPerSec  int64
	// Disks is the per-device view of the same interval (see pooldiag.go): empty
	// when the counters are not available.
	Disks []DiskSample
	// GoHeapLiveBytes / GoGCCPUPercent / GoMemLimitBytes are the server's own Go
	// runtime over the interval (see goruntime.go): the heap the last collection
	// found reachable, the share of the runtime's CPU time spent collecting, and
	// GOMEMLIMIT. Zero when the toolchain does not expose them.
	GoHeapLiveBytes int64
	GoGCCPUPercent  float64
	GoMemLimitBytes int64
}

// ResourceStats is what a whole tier consumed: the average is what a viewer
// costs in steady state, the peak is what the machine had to absorb.
type ResourceStats struct {
	CPUPercentAvg, CPUPercentPeak                     float64
	MachineCPUPercentAvg, MachineCPUPercentPeak       float64
	MachineIOWaitPercentAvg, MachineIOWaitPercentPeak float64
	MemUsedBytesAvg, MemUsedBytesMax                  int64
	ProcessRSSBytesAvg, ProcessRSSBytesMax            int64
	MemCacheBytesAvg                                  int64
	MemLimitBytes                                     int64
	DiskReadBytesPerSecAvg                            int64
	DiskReadMeasured                                  bool
	AppReadBytesPerSecAvg                             int64
	AppReadMeasured                                   bool
	ServedBytesPerSecAvg                              int64
	Samples                                           int
	// Disks is what each physical drive did over the tier, busiest first.
	Disks []DiskStat
	// The server's Go runtime over the tier (see goruntime.go).
	GoHeapLiveBytesAvg, GoHeapLiveBytesMax int64
	GoGCCPUPercentAvg, GoGCCPUPercentPeak  float64
	GoMemLimitBytes                        int64
}

// counterSource is where the raw monotonic counters come from. The server's
// own cgroup is the only implementation that ships; the seam exists so a test
// can model a machine's load curve directly instead of racing a goroutine
// against the kernel's.
type counterSource interface {
	// cpuUsageUsec is cumulative CPU time consumed, in microseconds.
	cpuUsageUsec() int64
	// memory is current usage and the ceiling it is measured against.
	// memory returns what the process genuinely holds (anon+kernel), the page
	// cache it has accumulated, and the ceiling. Cache is NOT consumption.
	memory() (used, cache, limit int64)
	// diskReadBytes is cumulative bytes the BLOCK LAYER actually fetched from
	// the devices the library sits on - what the hardware really did, whoever
	// asked. measured is false when no device could be tied to the library.
	diskReadBytes() (bytes int64, measured bool)
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

// machineSource is the optional seam for the whole-machine readings: a test
// machine that models only the container keeps compiling and reports none.
type machineSource interface {
	machineTicks() machineTicks
	processRSS() int64
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
	lastDisks     map[string]diskCounters
	lastGo        goRuntime
	lastMachine   machineTicks
	appMeasured   bool
}

func newResourceSampler(src counterSource) *resourceSampler {
	return &resourceSampler{src: src, ncpu: src.cpuAllowance()}
}

// sample reads the current counters and returns the rates since the previous
// call. servedTotal is the running total of bytes the viewers have pulled.
// ok is false for the very first call (no previous reading to diff against).
func (s *resourceSampler) sample(servedTotal int64, now time.Time) (ResourceSample, bool) {
	cpuUsec := s.src.cpuUsageUsec()
	diskRead, diskMeasured := s.src.diskReadBytes()
	appRead := s.src.appReadBytes()
	memUsed, memCache, memLimit := s.src.memory()
	var disks map[string]diskCounters
	if ds, ok := s.src.(diskActivitySource); ok {
		disks = ds.diskCounters()
	}
	prevDisks := s.lastDisks
	s.lastDisks = disks
	var machine machineTicks
	var rss int64
	if ms, ok := s.src.(machineSource); ok {
		machine = ms.machineTicks()
		rss = ms.processRSS()
	}
	prevMachine := s.lastMachine
	s.lastMachine = machine
	goNow := readGoRuntime()
	prevGo := s.lastGo
	s.lastGo = goNow

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
		MemUsedBytes:     memUsed,
		MemCacheBytes:    memCache,
		MemLimitBytes:    memLimit,
		ProcessRSSBytes:  rss,
		DiskReadMeasured: diskMeasured,
	}
	out.MachineCPUPercent, out.MachineIOWaitPercent = machineDeltas(prevMachine, machine)
	if cpuUsec > 0 && prevCPU > 0 && s.ncpu > 0 {
		// usage is in microseconds of CPU time; the wall clock gives the
		// budget (elapsed x cores), so the ratio is the share of the machine.
		out.CPUPercent = float64(cpuUsec-prevCPU) / (elapsed * 1e6 * s.ncpu) * 100
	}
	if diskMeasured && diskRead >= prevDisk {
		out.DiskReadBytesPerSec = int64(float64(diskRead-prevDisk) / elapsed)
	}
	// The in-process counter starts at zero and only moves once a viewer
	// reads: it is "measured" from the first byte it ever counted, and stays so.
	if appRead > 0 {
		s.appMeasured = true
	}
	out.AppReadMeasured = s.appMeasured
	if appRead > 0 && prevApp > 0 && appRead >= prevApp {
		out.AppReadBytesPerSec = int64(float64(appRead-prevApp) / elapsed)
	}
	if servedTotal >= prevServed {
		out.ServedBytesPerSec = int64(float64(servedTotal-prevServed) / elapsed)
	}
	out.Disks = diskDeltas(prevDisks, disks, elapsed)
	out.GoHeapLiveBytes = goNow.heapLiveBytes
	out.GoMemLimitBytes = goNow.memLimitBytes
	out.GoGCCPUPercent = gcCPUPercent(prevGo, goNow)
	return out, true
}

// minJudgeSamples is the least evidence a MEASURED tier may be judged on. One
// or two samples is a snapshot, not a regime - and a snapshot taken while an
// audience recovers is how a healthy tier gets condemned.
const minJudgeSamples = 3

// regimeResumeFraction / regimeBurstCeiling bound, as multiples of the
// reference per-viewer throughput, the band a freshly resized audience must
// come back INTO before its tier's measurement window opens. The gate is
// two-sided because the transient is:
//
//   - BELOW the band, the viewers are still restarting - a rebuilt audience
//     pulls a fraction of its regime for many seconds, and samples taken then
//     condemned every tier above 21 in the 2026-07-14 run;
//   - ABOVE it, they are burst-filling their buffers all at once - more than
//     twice any regime, and a window that opened on "at least half the
//     reference" quoted 468 Mb/s at tier 21 against a 217 Mb/s regime in the
//     2026-07-16 run.
//
// The bounds are deliberately loose: a genuine plateau stays inside them (N
// viewers' worth of bytes over N+k heads is well above half), while a restart
// (a tenth of the regime) and a burst (double it and more) do not.
const (
	regimeResumeFraction = 0.5
	regimeBurstCeiling   = 1.5
)

// regimeWaitCapFactor bounds how long the window waits for the regime, as a
// multiple of the tier's warm-up, measured from the tier's start. An audience
// that NEVER comes back is the tier's own verdict, and the run must measure
// that failure rather than wait forever. (A zero warm-up - unit tests - makes
// the cap zero too: the gate is then inert by construction.)
const regimeWaitCapFactor = 3

// resourceState accumulates samples per tier, skipping each tier's warm-up,
// and tracks how close to saturation the machine currently is.
type resourceState struct {
	byTier map[int][]ResourceSample

	// measured[t] records that tier t was held under a FULL measurement window.
	// Only such tiers may anchor a throughput verdict, appear in the report's
	// table, or feed the per-viewer slope: a FAST hold is hunting, not evidence -
	// its samples sit inside a ten-viewer join burst that pulls several times any
	// regime and burns CPU no steady state ever will.
	measured map[int]bool

	// tierStartedAt is when the current tier gained its viewer; samples before
	// tierStartedAt+warmup are start-up transients, not steady state.
	tierStartedAt time.Time
	warmup        time.Duration
	lastTier      int

	// The regime gate. After a resize - and above all after a REBUILD, where the
	// whole audience restarts at once - the fixed warm-up is not enough: the
	// viewers may still be climbing back to their per-viewer rate when it
	// expires, and every sample taken then describes the restart, not the tier.
	// The gate keeps the window shut until the audience has actually resumed
	// (or a hard cap passes), using the last measured regime as the yardstick.
	curTierMeasured    bool
	regimeOpen         bool
	regimeRefPerViewer float64

	// baselineSamples is what the server consumed BEFORE any viewer joined,
	// read over baselineFor: the floor an operator reads every cost against,
	// and the disks' wait at rest that the saturation verdict is measured from.
	baselineSamples []ResourceSample

	// windows is every hold the run made, in order: the climb as it happened,
	// FAST hunts and broken tiers included. cur is the one open now.
	windows []*windowRecord
	cur     *windowRecord

	cpuOverRun, memOverRun int
}

// windowRecord is one hold of one tier: what the machine did while the tier
// was observed, and how the tier fared. A tier measured twice (a break that
// had to reproduce) is two windows, and the table shows both.
type windowRecord struct {
	id        int
	tier      int
	phase     string
	measured  bool
	startedAt time.Time
	endedAt   time.Time
	samples   []ResourceSample
	closed    bool
	broke     bool
	reason    StopReason
	detail    string
}

// addBaseline records one pre-viewer sample.
func (rs *resourceState) addBaseline(s ResourceSample) {
	rs.baselineSamples = append(rs.baselineSamples, s)
}

// baseline is the idle server, averaged over the pre-viewer window.
func (rs *resourceState) baseline() ResourceStats {
	return statsOf(rs.baselineSamples)
}

// windowID is the id of the hold open now; 0 before any tier started.
func (rs *resourceState) windowID() int {
	if rs.cur == nil {
		return 0
	}
	return rs.cur.id
}

// openWindow closes the hold in progress (as held, unless closeWindow already
// said otherwise) and opens a new one for tier.
func (rs *resourceState) openWindow(tier int, phase string, measured bool, now time.Time) {
	rs.closeWindow(now, "", "")
	w := &windowRecord{id: len(rs.windows) + 1, tier: tier, phase: phase, measured: measured, startedAt: now}
	rs.windows = append(rs.windows, w)
	rs.cur = w
}

// nextWindow opens another window of the SAME audience size: the tier's
// record and its warm-up clock are kept, only the window changes.
func (rs *resourceState) nextWindow(tier int, now time.Time, phase string) {
	rs.closeWindow(now, "", "")
	w := &windowRecord{id: len(rs.windows) + 1, tier: tier, phase: phase, measured: rs.curTierMeasured, startedAt: now}
	rs.windows = append(rs.windows, w)
	rs.cur = w
	rs.cpuOverRun, rs.memOverRun = 0, 0
}

// closeWindow ends the hold in progress with its verdict: reason names what
// broke it, "" means it held. A second call on a closed window is a no-op, so
// the run loop may close at the verdict and openWindow may close for safety.
func (rs *resourceState) closeWindow(now time.Time, reason StopReason, detail string) {
	w := rs.cur
	if w == nil || w.closed {
		return
	}
	w.closed, w.endedAt = true, now
	if reason != "" {
		w.broke, w.reason, w.detail = true, reason, detail
	}
}

func newResourceState(warmup time.Duration, startedAt time.Time) *resourceState {
	return &resourceState{
		byTier:        map[int][]ResourceSample{},
		measured:      map[int]bool{},
		tierStartedAt: startedAt,
		warmup:        warmup,
		regimeOpen:    true,
	}
}

// settling reports that the tier is not yet in a measurable state: either
// still inside its warm-up window, or - for a measured tier - its audience has
// not resumed the regime after the resize. Viewers that just (re)joined are
// burst-filling or recovering, and the latency they see (and impose) is a
// transient, not what the tier costs in steady state. Judging a tier on those
// samples condemns tiers that would have held.
func (rs *resourceState) settling(now time.Time) bool {
	return now.Sub(rs.tierStartedAt) < rs.warmup || !rs.regimeOpen
}

// startTier begins a new tier's warm-up RIGHT NOW, rather than whenever the
// next resource sample happens to land.
//
// The sampler ticks every couple of seconds; leaving it to notice the change
// means the tier's first seconds are judged with no warm-up at all - and those
// are exactly the seconds when requests issued under the PREVIOUS (overloaded)
// tier are still landing. A tier stepped back down from an overload would
// inherit its slow fetches and be condemned for a stall it never had.
//
// measuredHold says whether the hold about to start is a full measurement
// window (BISECT/CONFIRM/SETTLE) rather than a FAST hunt. A measured hold
// starts the tier's record afresh - the tier number may have been visited on a
// FAST hold, and a join burst must never sit inside a measured tier's stats -
// and arms the regime gate against the best regime measured so far.
func (rs *resourceState) startTier(tier int, now time.Time, measuredHold bool, phase string) {
	rs.lastTier = tier
	rs.tierStartedAt = now
	rs.cpuOverRun, rs.memOverRun = 0, 0
	rs.curTierMeasured = measuredHold
	rs.regimeOpen = true
	rs.regimeRefPerViewer = 0
	rs.openWindow(tier, phase, measuredHold, now)
	if !measuredHold {
		return
	}
	delete(rs.byTier, tier)
	rs.measured[tier] = true
	if ref := rs.regimeReference(tier); ref > 0 {
		rs.regimeRefPerViewer = ref
		rs.regimeOpen = false
	}
}

// regimeReference is the per-viewer rate of the best-served tier measured so
// far - the yardstick a freshly resized audience must climb back to before its
// own window opens. Any measured tier will do (the bisection walks down as
// well as up); zero when nothing has been measured yet, which leaves the gate
// open: the first measured tier IS the first regime.
func (rs *resourceState) regimeReference(excluding int) float64 {
	var best float64
	for t := range rs.byTier {
		if t <= 0 || t == excluding || !rs.measured[t] || len(rs.byTier[t]) == 0 {
			continue
		}
		if pv := float64(rs.stats(t).ServedBytesPerSecAvg) / float64(t); pv > best {
			best = pv
		}
	}
	return best
}

// setWarmup changes how much of a tier is discarded as transient. The climb
// hunts on a short hold (where a long warm-up would leave no samples at all)
// and judges on a long one, so the warm-up follows the phase.
func (rs *resourceState) setWarmup(d time.Duration) { rs.warmup = d }

// minGainFraction is the share of the EXPECTED throughput gain a tier must
// actually deliver for its extra viewers to count as served.
//
// It is a fraction of the expected gain, not a flat percentage, and that
// distinction is the whole point. Viewers are paced in real time: each pulls its
// title's bitrate and no more, so N+1 viewers move (N+1)/N times the bytes of N
// viewers. The gain a single extra viewer can possibly produce is therefore 1/N,
// and it SHRINKS as the audience grows.
//
// A flat 5% threshold (what this used to be) is thus arithmetically unreachable
// past 20 viewers: going from 21 to 22 can only ever add 1/21 = 4.76% of
// throughput, which is less than 5% no matter how idle the machine is. The run
// declared "throughput plateaued - the server cannot read the media any faster",
// blamed the storage, and stopped - on a box at 25% CPU, 36% memory and 48% of
// its measured disk bandwidth. It did so at the same ~21 viewers on a one-disk
// dev box and a two-disk production server, because the number was never coming
// from the hardware: it was coming from this constant.
//
// Measured against the expected gain instead, the test asks the only question
// that means anything: when the audience grew by this much, did the throughput
// grow with it? Half of the expected gain is the bar - generous enough to absorb
// a lighter film in the mix, unforgiving of a server that has genuinely stopped
// scaling.
const minGainFraction = 0.5

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
	// A size judged on several windows is judged on each window's own
	// throughput: the tier's running average would hand every later window
	// the verdict of the first.
	if w := rs.cur; w != nil && w.tier == tier && w.measured {
		if ws := statsOf(w.samples); ws.Samples > 0 {
			cur = ws
		}
	}
	// Only a tier measured under a full window may be judged, and only against
	// references measured the same way: a FAST hold's samples sit inside a join
	// burst that pulls several times any regime, and a verdict comparing a
	// regime against a burst measures the burst.
	if cur.Samples == 0 || cur.ServedBytesPerSecAvg <= 0 || tier <= 0 || !rs.measured[tier] {
		return false, 0
	}
	// A verdict that names the storage needs the storage to have DONE something.
	// When the disk counter sat at (nearly) zero - every byte from the page
	// cache, or reads the kernel cannot see (9p, network shares) - "the server
	// cannot read the media any faster" is a fabrication, whatever the curve
	// looks like. Whatever limits the box then, it was not seen on the storage.
	if diskReadStarved(cur) {
		return false, 0
	}
	best, bestTier := rs.bestMeasuredServed(int(float64(tier) / plateauMinHeadcountRatio))
	// A plateau only means something once real data is moving. Below this, the
	// run is measuring noise - and calling a trickle "the storage ceiling" would
	// blame the disk for whatever else is actually wrong.
	if best < minPlateauBytesPerSec || bestTier <= 0 {
		return false, 0
	}
	// Real-time pacing means throughput should scale with the headcount: going
	// from bestTier to tier viewers should move tier/bestTier times the bytes.
	// Demand at least half of that. A server still scaling clears this easily; a
	// server that has stopped scaling cannot.
	expectedGain := float64(tier)/float64(bestTier) - 1
	required := float64(best) * (1 + expectedGain*minGainFraction)
	if float64(cur.ServedBytesPerSecAvg) < required {
		return true, best
	}
	return false, 0
}

// plateauMinHeadcountRatio is how much larger an audience must be than the
// reference it is compared with before "it pushed no more bytes" means
// anything. Near the end of a bisection two sizes are a few percent apart
// (65 and 67 viewers, 2026-10-16): the gain expected between them is smaller
// than the window-to-window spread of the throughput, and the comparison flagged
// a plateau on noise. Fifteen percent more viewers is a gain the spread cannot
// hide.
const plateauMinHeadcountRatio = 1.15

// bestMeasuredServed is the highest served throughput among the tiers AT OR
// BELOW `limit` that were measured under a full window, and the headcount that
// produced it: the reference a plateau verdict compares against. The gain
// expected of the current tier is only meaningful relative to the headcount
// that produced the figure being compared against.
func (rs *resourceState) bestMeasuredServed(limit int) (int64, int) {
	var best int64
	bestTier := 0
	for t, samples := range rs.byTier {
		if t <= 0 || t > limit || !rs.measured[t] || len(samples) == 0 {
			continue
		}
		if served := rs.stats(t).ServedBytesPerSecAvg; served > best {
			best, bestTier = served, t
		}
	}
	return best, bestTier
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
		// The ramp moved without startTier (a spawn/shed outside a tier
		// boundary): this tier is in transition, warm up again - as an
		// unmeasured hold, since nothing armed a measurement window for it.
		rs.lastTier = tier
		rs.tierStartedAt = now
		rs.cpuOverRun, rs.memOverRun = 0, 0
		rs.curTierMeasured = false
		rs.regimeOpen = true
		if tier > 0 {
			rs.openWindow(tier, phaseFast.String(), false, now)
		}
	}
	if tier == 0 {
		rs.addBaseline(s)
		return "", ""
	}
	if s.MemLimitBytes > 0 && float64(s.MemUsedBytes)/float64(s.MemLimitBytes)*100 >= memCriticalPercent {
		return StopMemorySaturated, formatMemDetail(s.MemUsedBytes, s.MemLimitBytes, tier)
	}
	if now.Sub(rs.tierStartedAt) < rs.warmup {
		return "", ""
	}
	if !rs.regimeOpen {
		// The audience is not yet back at its regime after the resize: below
		// the band its viewers are still restarting, above it they are
		// burst-filling their buffers, and either way the samples describe the
		// transient, not the tier. The window opens when the per-viewer rate is
		// back INSIDE the neighbourhood of the last measured regime - or at the
		// hard cap, past which the failure to resume IS the tier's evidence and
		// gets measured like anything else.
		perViewer := float64(s.ServedBytesPerSec) / float64(tier)
		switch {
		case perViewer >= rs.regimeRefPerViewer*regimeResumeFraction &&
			perViewer <= rs.regimeRefPerViewer*regimeBurstCeiling:
			rs.regimeOpen = true
		case now.Sub(rs.tierStartedAt) >= regimeWaitCapFactor*rs.warmup:
			rs.regimeOpen = true
		default:
			return "", ""
		}
	}
	rs.byTier[tier] = append(rs.byTier[tier], s)
	if rs.cur != nil && rs.cur.tier == tier && !rs.cur.closed {
		rs.cur.samples = append(rs.cur.samples, s)
	}

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

// tiers lists every tier measured under a full window, smallest first. It is
// what lets the report show the CLIMB rather than only its last rung: an
// operator reading one row per audience size can see the cost per viewer hold
// steady (the server is scaling) or bend upward (it is not), which a single
// final number hides completely.
//
// FAST holds are excluded deliberately: their samples sit inside a join burst,
// and one row at 61% CPU (the burst) next to rows at 1-9% (regimes) makes the
// whole table - above all the per-viewer column an operator sizes a machine
// with - unusable.
func (rs *resourceState) tiers() []int {
	out := make([]int, 0, len(rs.byTier))
	for t, samples := range rs.byTier {
		if t > 0 && rs.measured[t] && len(samples) > 0 {
			out = append(out, t)
		}
	}
	sort.Ints(out)
	return out
}

// tierCosts turns the measured tiers into the report's table, one row per
// audience size. serverShare scales every CPU figure down to the server's own
// contribution, so the table never charges the load generator's overhead to the
// machine an operator is trying to size.
func (rs *resourceState) tierCosts(serverShare float64) []TierCost {
	tiers := rs.tiers()
	out := make([]TierCost, 0, len(tiers))
	for _, t := range tiers {
		s := rs.stats(t)
		cpu := s.CPUPercentAvg * serverShare
		out = append(out, TierCost{
			Viewers:             t,
			CPUPercent:          cpu,
			MachineCPUPercent:   s.MachineCPUPercentAvg,
			MachineIOWaitPct:    s.MachineIOWaitPercentAvg,
			MemUsedBytes:        s.MemUsedBytesAvg,
			ProcessRSSBytes:     s.ProcessRSSBytesAvg,
			DiskReadBytesPerSec: s.DiskReadBytesPerSecAvg,
			ServedBytesPerSec:   s.ServedBytesPerSecAvg,
			Samples:             s.Samples,
			GoHeapLiveBytes:     s.GoHeapLiveBytesAvg,
			GoGCCPUPercent:      s.GoGCCPUPercentAvg,
		})
	}
	return out
}

func (rs *resourceState) stats(tier int) ResourceStats {
	return statsOf(rs.byTier[tier])
}

// statsOf averages a run of samples into one ResourceStats.
func statsOf(samples []ResourceSample) ResourceStats {
	if len(samples) == 0 {
		return ResourceStats{}
	}
	var out ResourceStats
	var cpuSum, memSum, cacheSum, diskSum, appSum, servedSum, heapSum, gcSum float64
	var machineSum, iowaitSum, rssSum float64
	for _, s := range samples {
		machineSum += s.MachineCPUPercent
		iowaitSum += s.MachineIOWaitPercent
		rssSum += float64(s.ProcessRSSBytes)
		out.MachineCPUPercentPeak = max(out.MachineCPUPercentPeak, s.MachineCPUPercent)
		out.MachineIOWaitPercentPeak = max(out.MachineIOWaitPercentPeak, s.MachineIOWaitPercent)
		out.ProcessRSSBytesMax = max(out.ProcessRSSBytesMax, s.ProcessRSSBytes)
		out.DiskReadMeasured = out.DiskReadMeasured || s.DiskReadMeasured
		out.AppReadMeasured = out.AppReadMeasured || s.AppReadMeasured
		heapSum += float64(s.GoHeapLiveBytes)
		gcSum += s.GoGCCPUPercent
		out.GoHeapLiveBytesMax = max(out.GoHeapLiveBytesMax, s.GoHeapLiveBytes)
		out.GoGCCPUPercentPeak = max(out.GoGCCPUPercentPeak, s.GoGCCPUPercent)
		out.GoMemLimitBytes = max(out.GoMemLimitBytes, s.GoMemLimitBytes)
		cpuSum += s.CPUPercent
		memSum += float64(s.MemUsedBytes)
		cacheSum += float64(s.MemCacheBytes)
		diskSum += float64(s.DiskReadBytesPerSec)
		appSum += float64(s.AppReadBytesPerSec)
		servedSum += float64(s.ServedBytesPerSec)
		out.CPUPercentPeak = max(out.CPUPercentPeak, s.CPUPercent)
		out.MemUsedBytesMax = max(out.MemUsedBytesMax, s.MemUsedBytes)
		out.MemLimitBytes = max(out.MemLimitBytes, s.MemLimitBytes)
	}
	n := float64(len(samples))
	out.MachineCPUPercentAvg = machineSum / n
	out.MachineIOWaitPercentAvg = iowaitSum / n
	out.ProcessRSSBytesAvg = int64(rssSum / n)
	out.CPUPercentAvg = cpuSum / n
	out.MemUsedBytesAvg = int64(memSum / n)
	out.MemCacheBytesAvg = int64(cacheSum / n)
	out.DiskReadBytesPerSecAvg = int64(diskSum / n)
	out.AppReadBytesPerSecAvg = int64(appSum / n)
	out.ServedBytesPerSecAvg = int64(servedSum / n)
	out.Samples = len(samples)
	out.Disks = aggregateDisks(samples)
	out.GoHeapLiveBytesAvg = int64(heapSum / n)
	out.GoGCCPUPercentAvg = gcSum / n
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
		// Only tiers measured under a full window carry a cost worth reading a
		// slope from; a FAST hold's samples are a join burst.
		if !rs.measured[tier] || len(rs.byTier[tier]) == 0 {
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

// cgroupSource reads the server's own cgroup, and the machine beside it.
// fsRoot is "/" in production; tests point it at a staged tree. devices are
// the whole disks the library was mapped to (see devices_linux.go): the disk
// figure is their /proc/diskstats sum, and absent when none was mapped.
// appRead counts, in-process, the bytes the packager read from its sources;
// nil when nothing counts them.
type cgroupSource struct {
	fsRoot  string
	devices []string
	appRead func() int64
}

func (c cgroupSource) cpuUsageUsec() int64 { return readCPUUsageUsec(c.fsRoot) }
func (c cgroupSource) diskReadBytes() (int64, bool) {
	if len(c.devices) == 0 {
		return 0, false
	}
	counters := readDiskCounters(c.fsRoot)
	var total int64
	for _, d := range c.devices {
		total += int64(counters[d].sectorsRead) * 512 //nolint:gosec // sectors read, far below the int64 range
	}
	return total, true
}
func (c cgroupSource) appReadBytes() int64 {
	if c.appRead == nil {
		return 0
	}
	return c.appRead()
}
func (c cgroupSource) memory() (used, cache, limit int64) { return readMemory(c.fsRoot) }
func (c cgroupSource) cpuAllowance() float64              { return cpuAllowance(c.fsRoot) }
func (c cgroupSource) machineTicks() machineTicks         { return readMachineTicks(c.fsRoot) }
func (c cgroupSource) processRSS() int64                  { return readProcessRSS(c.fsRoot) }

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

// readMemory returns what the server actually COSTS in memory, its page cache,
// and its ceiling. An unlimited cgroup ("max") falls back to the machine's total
// RAM, which IS the real ceiling in that case.
//
// `used` is the cgroup's ANONYMOUS memory - heap, stacks, the bytes the process
// genuinely holds - and never memory.current, which is a trap for exactly this
// measurement: it counts the PAGE CACHE of every media file the run has read.
// A load test reads films by the gigabyte, so memory.current climbs to whatever
// the box will let the cache grow to and then reports it as consumption. One run
// showed 4.3 GB "used" by 21 viewers on a packager measured at under a megabyte
// per viewer. It was the films, sitting in cache, reclaimable the instant anyone
// wanted the RAM back - so it is `cache` here, reported apart, and it never
// counts toward saturation.
func readMemory(fsRoot string) (used, cache, limit int64) {
	if raw, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/memory.stat")); err == nil {
		for line := range strings.Lines(string(raw)) {
			key, val, ok := strings.Cut(strings.TrimSpace(line), " ")
			if !ok {
				continue
			}
			v, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				continue
			}
			switch key {
			case "anon":
				used += v
			// The kernel's own structures for this cgroup are a real cost the
			// process imposes, unlike the file cache.
			case "kernel":
				used += v
			case "file":
				cache = v
			}
		}
	}
	if used == 0 {
		// No memory.stat (cgroup v1, or an unreadable one): memory.current is a
		// poor answer, but a wrong number here is better than none - the report
		// would otherwise claim the server uses no memory at all.
		if raw, err := os.ReadFile(filepath.Join(fsRoot, "sys/fs/cgroup/memory.current")); err == nil {
			if v, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64); err == nil {
				used = v
			}
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
	return used, cache, limit
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
