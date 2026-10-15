package loadtest

import (
	"strings"
	"time"
)

// verdict.go - what the measurements say about the storage, read against the
// server at rest rather than against a fixed percentage.
//
// "Busy 80 % of the time" was the old bar for a saturated disk, and it missed
// every break the 2026-10-15 cross-check reproduced: the tiers that stalled
// had their disks at 56-82 % busy, under the bar, while every read on them
// waited 27 to 46 ms - three to six times the 7-8 ms a read took at the tier
// that held, with the machine spending 63-81 % of its time in iowait. A disk
// is saturated when its queue has formed, and the queue shows in the time a
// read waits, not in the share of seconds the disk was doing something.

const (
	// diskAwaitSaturatedMs is the mean wait per read past which the queue has
	// formed. Sits between the held tier's 7-8 ms and the broken tiers' 27 ms
	// of the cross-check, with room on both sides for the run-to-run spread.
	diskAwaitSaturatedMs = 20.0
	// diskAwaitBaselineFactor is how much longer than at rest a read must wait,
	// on top of the floor above: a pool that already waits 15 ms idle (SMR, a
	// busy NAS) is not saturated at 20.
	diskAwaitBaselineFactor = 2.5
	// diskAwaitBaselineFloorMs is the least rest wait the factor is applied
	// to: an SSD rests under a millisecond, and 2.5 times nothing would make
	// the floor the only bar - which is what it should be there.
	diskAwaitBaselineFloorMs = 4.0
	// diskSaturatedMinUtilPct is the least share of the interval the device
	// must have been busy: a disk slow on a handful of reads but idle most of
	// the time is not what held the audience back.
	diskSaturatedMinUtilPct = 30.0
	// iowaitCorroboratesPct is the machine-wide iowait share that corroborates
	// the device's own wait: the cores were parked on this storage.
	iowaitCorroboratesPct = 25.0
)

// DiskVerdict is the busiest device of a hold, judged.
type DiskVerdict struct {
	Device string
	// AwaitMs is the mean wait per read over the hold, AwaitBaselineMs the same
	// device's wait before the first viewer joined, AwaitMsPeak the worst
	// one-second interval.
	AwaitMs, AwaitBaselineMs, AwaitMsPeak float64
	// QueueDepth is the mean requests in flight (aqu-sz), UtilPct the share of
	// time busy, ReadBytesPerSec what it delivered.
	QueueDepth, UtilPct float64
	ReadBytesPerSec     int64
	// IOWaitPct is the machine's share of time waiting on I/O over the hold.
	IOWaitPct float64
	// Saturated is the verdict: the wait crossed the bar, with the device busy
	// or the machine in iowait to corroborate it.
	Saturated bool
}

// diskVerdict judges the device that waited longest over a hold against the
// baseline. Zero value when the hold saw no device.
func diskVerdict(hold ResourceStats, baseline ResourceStats) DiskVerdict {
	var top *DiskStat
	for i := range hold.Disks {
		d := &hold.Disks[i]
		if top == nil || d.AwaitMsAvg > top.AwaitMsAvg || (d.AwaitMsAvg == top.AwaitMsAvg && d.UtilPctAvg > top.UtilPctAvg) {
			top = d
		}
	}
	if top == nil {
		return DiskVerdict{}
	}
	v := DiskVerdict{
		Device:          top.Device,
		AwaitMs:         top.AwaitMsAvg,
		AwaitMsPeak:     top.AwaitMsPeak,
		QueueDepth:      top.QueueDepthAvg,
		UtilPct:         top.UtilPctAvg,
		ReadBytesPerSec: top.ReadBytesPerSecAvg,
		IOWaitPct:       hold.MachineIOWaitPercentAvg,
	}
	for _, d := range baseline.Disks {
		if d.Device == top.Device {
			v.AwaitBaselineMs = d.AwaitMsAvg
		}
	}
	bar := max(diskAwaitSaturatedMs, diskAwaitBaselineFactor*max(v.AwaitBaselineMs, diskAwaitBaselineFloorMs))
	v.Saturated = v.AwaitMs >= bar && (v.UtilPct >= diskSaturatedMinUtilPct || v.IOWaitPct >= iowaitCorroboratesPct)
	return v
}

// TierWindow is one hold of the climb, as the report shows it: the tier, how
// it was observed, whether it held, and what the machine did meanwhile. A
// tier measured twice (a break that had to reproduce) is two rows.
type TierWindow struct {
	Tier int
	// Phase is the climb phase the hold served ("ramping", "narrowing",
	// "confirming", "measuring"); Measured says it was a full window rather
	// than a FAST hunt.
	Phase    string
	Measured bool
	Broke    bool
	Reason   StopReason
	Detail   string
	// Resources is what the machine did over the hold; CPU is the server's
	// share.
	Resources ResourceStats
	// SegmentMsP50 / SegmentMsP95 are the raw fetch times of the hold's
	// segments; SegmentMsP95Scaled is the p95 of the same fetches scaled to the
	// reference segment length - the figure the stall verdict is read on.
	SegmentMsP50, SegmentMsP95, SegmentMsP95Scaled int
	SegmentSamples                                 int
	TTFFMsP95                                      int
	DiskVerdict                                    DiskVerdict
	StartedAt, EndedAt                             time.Time
}

// windowReports turns every hold into its row. serverShare scales the CPU to
// the server's own; percentiles reads the hold's latency samples by window id.
func (rs *resourceState) windowReports(serverShare float64, percentiles func(windowID int) (p50, p95, p95Scaled, n, ttffP95 int), baseline ResourceStats) []TierWindow {
	out := make([]TierWindow, 0, len(rs.windows))
	for _, w := range rs.windows {
		res := statsOf(w.samples)
		res.CPUPercentAvg *= serverShare
		res.CPUPercentPeak *= serverShare
		tw := TierWindow{
			Tier: w.tier, Phase: w.phase, Measured: w.measured,
			Broke: w.broke, Reason: w.reason, Detail: w.detail,
			Resources:   res,
			DiskVerdict: diskVerdict(res, baseline),
			StartedAt:   w.startedAt, EndedAt: w.endedAt,
		}
		if percentiles != nil {
			tw.SegmentMsP50, tw.SegmentMsP95, tw.SegmentMsP95Scaled, tw.SegmentSamples, tw.TTFFMsP95 = percentiles(w.id)
		}
		out = append(out, tw)
	}
	return out
}

// What gives out first, in the words the report uses: the panel turns each
// into a plain sentence an operator understands without the details.
const (
	LimiterDisk    = "disk"    // the disk that holds the films
	LimiterCPU     = "cpu"     // the processor
	LimiterMemory  = "memory"  // the memory
	LimiterProxy   = "proxy"   // the reverse proxy in front of the server
	LimiterServer  = "server"  // the server answered with errors
	LimiterUnknown = "unknown" // nothing on this machine was saturated
	LimiterNone    = "none"    // nothing gave out (the operator's ceiling)
)

// Advice the report may give, only when one follows from the measurements.
const (
	// AdviceSpreadTitles: one disk saturated while another disk of the
	// library sat mostly idle - spreading the most watched titles over the
	// disks moves the ceiling.
	AdviceSpreadTitles = "spread_titles"
	// AdviceTryDirect: the run went through the proxy and nothing on the
	// server was saturated - a run without it says what the server holds.
	AdviceTryDirect = "try_direct"
)

// spreadIdleUtilPct is the busy share under which another disk counts as
// having room to take titles.
const spreadIdleUtilPct = 40.0

// limiterOf names what gave out first, where (a library location for a disk),
// and the advice that follows, if any. breakRes is what the machine did under
// the first audience that did not hold; verdict judges its busiest disk.
func limiterOf(stop StopReason, breakDetail string, breakRes ResourceStats, verdict DiskVerdict, viaProxy bool, devices []DeviceMapping) (limiter, location, advice string) {
	switch stop {
	case StopMaxViewers, StopCompleted:
		return LimiterNone, "", ""
	case StopCancelled, StopTokenExpired, StopError, StopLibraryLimit, "":
		return "", "", ""
	case StopMemorySaturated:
		return LimiterMemory, "", ""
	case StopCPUSaturated:
		return LimiterCPU, "", ""
	case StopServerFailing:
		if strings.Contains(breakDetail, "status=") && !strings.Contains(breakDetail, "status=0") {
			return LimiterServer, "", ""
		}
	}
	if verdict.Saturated {
		for _, m := range devices {
			if m.Device == verdict.Device {
				location = m.Location
				break
			}
		}
		if hasIdleOtherDisk(breakRes, verdict.Device, devices) {
			advice = AdviceSpreadTitles
		}
		return LimiterDisk, location, advice
	}
	if breakRes.CPUPercentPeak >= cpuSaturationPercent {
		return LimiterCPU, "", ""
	}
	if viaProxy {
		return LimiterProxy, "", AdviceTryDirect
	}
	return LimiterUnknown, "", ""
}

// hasIdleOtherDisk reports whether another library disk had room while the
// saturated one did not.
func hasIdleOtherDisk(res ResourceStats, saturated string, devices []DeviceMapping) bool {
	library := map[string]bool{}
	for _, m := range devices {
		if m.Device != "" {
			library[m.Device] = true
		}
	}
	if len(library) < 2 {
		return false
	}
	seen := map[string]bool{}
	for _, d := range res.Disks {
		if d.Device == saturated || !library[d.Device] {
			continue
		}
		seen[d.Device] = true
		if d.UtilPctAvg < spreadIdleUtilPct {
			return true
		}
	}
	// A library disk that never appeared in the samples read nothing at all.
	for dev := range library {
		if dev != saturated && !seen[dev] {
			return true
		}
	}
	return false
}
