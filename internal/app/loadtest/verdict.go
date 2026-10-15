package loadtest

import "time"

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
