package loadtest

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// pooldiag.go - what a pooled mount hides, made visible.
//
// A FUSE pool (mergerfs) collapses every disk behind it into one device: the
// kernel's stat() says "one disk", the cgroup io counter sees nothing, and a
// run that saturates ONE drive of three reports "disks_probed: 1" and a
// verdict nobody can explain. Two questions decide such a run and neither is
// answerable from inside the mount:
//
//   - WHERE do the titles the audience watched actually live (which branch)?
//     A pool is not a RAID: a file sits on one disk, so an audience that is 85%
//     on one branch is a one-disk audience whatever the pool's total is.
//   - HOW BUSY was each physical disk during the run? /proc/diskstats is
//     host-wide even inside a container, so it sees the drives this kernel
//     drives, whoever reads them. That also means it sees OTHER readers: the
//     figures describe the disks, not only this run, and the report says so.

// diskCounters is one block device's cumulative counters from /proc/diskstats.
type diskCounters struct {
	reads       uint64 // reads completed
	sectorsRead uint64 // 512-byte sectors
	readMs      uint64 // time spent reading
	ioTicksMs   uint64 // time the device had at least one request in flight
}

// diskActivitySource is the optional seam a counterSource implements to expose
// per-device counters. It is a separate interface so a test machine that models
// only CPU and memory keeps compiling and simply reports no disks.
type diskActivitySource interface {
	diskCounters() map[string]diskCounters
}

// DiskSample is one device's activity over one sampling interval.
type DiskSample struct {
	Device          string
	UtilPct         float64 // share of the interval the device had a request in flight
	ReadBytesPerSec int64
	AwaitMs         float64 // mean time one read took, queueing included
}

// DiskStat is one device's activity over a whole tier.
type DiskStat struct {
	Device             string
	UtilPctAvg         float64
	UtilPctPeak        float64
	ReadBytesPerSecAvg int64
	AwaitMsAvg         float64
}

// BranchShare is how many of the audience's viewers were placed on one branch
// of a pooled mount.
type BranchShare struct {
	Branch  string
	Viewers int
}

// diskDeltas turns two readings of the counters into per-interval samples for
// the devices present in both. A device that moved no data and was never busy
// is left out: a quiet disk is the absence of a line, not a line of zeros.
func diskDeltas(prev, cur map[string]diskCounters, elapsedSec float64) []DiskSample {
	if elapsedSec <= 0 {
		return nil
	}
	out := make([]DiskSample, 0, len(cur))
	for dev, c := range cur {
		p, ok := prev[dev]
		if !ok || c.sectorsRead < p.sectorsRead || c.ioTicksMs < p.ioTicksMs {
			continue // appeared mid-run or the counters wrapped: no usable delta
		}
		s := DiskSample{
			Device:          dev,
			UtilPct:         min(100, float64(c.ioTicksMs-p.ioTicksMs)/(elapsedSec*1000)*100),
			ReadBytesPerSec: int64(float64(c.sectorsRead-p.sectorsRead) * 512 / elapsedSec),
		}
		if dr := c.reads - p.reads; dr > 0 && c.readMs >= p.readMs {
			s.AwaitMs = float64(c.readMs-p.readMs) / float64(dr)
		}
		if s.ReadBytesPerSec == 0 && s.UtilPct < 1 {
			continue
		}
		out = append(out, s)
	}
	return out
}

// aggregateDisks averages per-sample device activity over a tier. The average
// of a device is taken over the samples it APPEARED in, so a drive that sat idle
// half the window is not diluted into looking half as busy as it was when used.
func aggregateDisks(samples []ResourceSample) []DiskStat {
	type acc struct {
		n                 int
		util, rate, await float64
		awaitN            int
		peak              float64
	}
	byDev := map[string]*acc{}
	for _, s := range samples {
		for _, d := range s.Disks {
			a := byDev[d.Device]
			if a == nil {
				a = &acc{}
				byDev[d.Device] = a
			}
			a.n++
			a.util += d.UtilPct
			a.rate += float64(d.ReadBytesPerSec)
			a.peak = max(a.peak, d.UtilPct)
			if d.AwaitMs > 0 {
				a.await += d.AwaitMs
				a.awaitN++
			}
		}
	}
	out := make([]DiskStat, 0, len(byDev))
	for dev, a := range byDev {
		st := DiskStat{
			Device:             dev,
			UtilPctAvg:         a.util / float64(a.n),
			UtilPctPeak:        a.peak,
			ReadBytesPerSecAvg: int64(a.rate / float64(a.n)),
		}
		if a.awaitN > 0 {
			st.AwaitMsAvg = a.await / float64(a.awaitN)
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UtilPctAvg != out[j].UtilPctAvg {
			return out[i].UtilPctAvg > out[j].UtilPctAvg
		}
		return out[i].Device < out[j].Device
	})
	return out
}

// readDiskCounters parses fsRoot/proc/diskstats, keeping whole disks only
// (partitions would double-count their parent; loop, ram and optical devices
// are not storage). A whole disk is a name that exists under /sys/block.
func readDiskCounters(fsRoot string) map[string]diskCounters {
	f, err := os.Open(filepath.Join(fsRoot, "proc/diskstats"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	out := map[string]diskCounters{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 13 {
			continue
		}
		name := fields[2]
		if skipDevice(name) {
			continue
		}
		if _, err := os.Stat(filepath.Join(fsRoot, "sys/block", name)); err != nil {
			continue
		}
		var c diskCounters
		c.reads, _ = strconv.ParseUint(fields[3], 10, 64)
		c.sectorsRead, _ = strconv.ParseUint(fields[5], 10, 64)
		c.readMs, _ = strconv.ParseUint(fields[6], 10, 64)
		c.ioTicksMs, _ = strconv.ParseUint(fields[12], 10, 64)
		out[name] = c
	}
	return out
}

func skipDevice(name string) bool {
	for _, p := range []string{"loop", "ram", "zram", "sr", "fd", "nbd"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// diskCounters on the real source reads the host's per-device counters.
func (c cgroupSource) diskCounters() map[string]diskCounters { return readDiskCounters(c.fsRoot) }

// branchTally counts the viewers a pool placed on each branch of a pooled
// mount. branchOf resolves a file to its branch ("" when the mount is not a
// pool or cannot say); results are cached per path because a title is handed
// out many times.
type branchTally struct {
	branchOf func(path string) string
	cache    map[string]string
	counts   map[string]int
}

func newBranchTally(branchOf func(string) string) *branchTally {
	return &branchTally{branchOf: branchOf, cache: map[string]string{}, counts: map[string]int{}}
}

func (b *branchTally) record(path string) {
	if b == nil || b.branchOf == nil || path == "" {
		return
	}
	br, ok := b.cache[path]
	if !ok {
		br = b.branchOf(path)
		b.cache[path] = br
	}
	if br != "" {
		b.counts[br]++
	}
}

func (b *branchTally) shares() []BranchShare {
	if b == nil || len(b.counts) == 0 {
		return nil
	}
	out := make([]BranchShare, 0, len(b.counts))
	for br, n := range b.counts {
		out = append(out, BranchShare{Branch: br, Viewers: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Viewers != out[j].Viewers {
			return out[i].Viewers > out[j].Viewers
		}
		return out[i].Branch < out[j].Branch
	})
	return out
}
