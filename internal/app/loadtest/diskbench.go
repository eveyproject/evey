package loadtest

import (
	"context"
	"math/rand/v2"
	"os"
	"sort"
	"sync"
	"syscall"
	"time"
)

// diskbench.go - measure how fast this box can actually READ its media.
//
// This is the number that decides a direct-play server's capacity, and the one
// no gauge reports. The kernel's per-cgroup I/O accounting (io.stat) only counts
// BLOCK devices: when the library lives on a network share, a 9p/virtiofs mount
// under a VM, or anything that isn't a plain disk, it reports a confident zero
// while gigabits stream out. Rather than shrug and print "not measurable", the
// run reads the files itself and times it. That works on any filesystem, because
// it measures the only thing that matters: how fast the bytes arrive.
//
// It measures PER DISK, because a real library is spread over several. Four
// drives do not behave like one big drive: they deliver roughly four times the
// throughput, but each one saturates on its own, and the film a viewer picks
// decides which. A single aggregate figure would hide the drive that gives out
// first - which is precisely the one an operator needs to know about.
//
// And it reads CONCURRENTLY, at scattered offsets - because that is what an
// audience does. A single sequential read flatters spinning disks and network
// mounts alike (readahead does the work); several viewers scrubbing different
// films in parallel is a different, and far more honest, question.

// diskBenchLevels are the concurrency levels the probe sweeps.
//
// One number is a lie. Storage does not have "a throughput": it has a CURVE.
// Real disks give more aggregate throughput as readers pile on, up to a point.
// Some storage does the opposite past a certain width and COLLAPSES - a 9p or
// virtiofs mount multiplexed over a single channel, an over-subscribed NFS
// export, a spinning disk thrashing its head between concurrent streams. On one
// such mount, measured: 4 readers gave 373 MiB/s at 31ms latency; 32 readers
// gave 28 MiB/s at SIX SECONDS. A probe that samples one width lands somewhere
// on that curve and reports it as "the disk's speed", which is worse than not
// measuring at all - it hides the exact thing that decides how many viewers the
// box can hold.
//
// So the probe sweeps, and the report says where it breaks.
//
// The sweep reaches 32 because a REAL audience does. Stopping at 16 while the
// box holds 21 viewers means the widest number ever measured is narrower than
// the load being explained - and the peak, which lands at 1 or 4 readers on a
// spinning disk, then gets reported as "the disk's speed". An operator reads
// that as headroom. It is not headroom: it is a figure his workload can never
// reach.
var diskBenchLevels = []int{1, 4, 8, 16, 32}

const (
	// diskBenchReadersPerDisk is the widest level - how many files may be read
	// at once from ONE disk.
	diskBenchReadersPerDisk = 32
	// diskBenchBytesPerReader is how much each reader pulls at each level. Large
	// enough to outrun readahead and any small cache, short enough that sweeping
	// three levels stays within a few seconds.
	diskBenchBytesPerReader = 16 << 20 // 16 MiB
	// diskBenchChunk is the read size, matching the order of magnitude a
	// packager pulls per segment.
	diskBenchChunk = 1 << 20 // 1 MiB
	// diskBenchMaxDisks bounds the probe on a library spread very wide.
	diskBenchMaxDisks = 8
	// diskBenchTimeout bounds the whole probe: on a truly crawling mount it must
	// not hold the run hostage - a partial measurement is still an answer.
	// Sized for the target box (a weak NAS on spinning rust), not the dev rig:
	// the full sweep pulls 16 MiB x (1+4+16+32) = 848 MiB per disk, which is
	// about nine seconds at 100 MB/s.
	diskBenchTimeout = 45 * time.Second
)

// DiskLevel is what a disk delivered at ONE concurrency width.
type DiskLevel struct {
	Readers     int
	BytesPerSec int64
	// WorstReadMs is the slowest single 1 MiB read at this width. This is what a
	// viewer feels: when it climbs into seconds, playback takes seconds to start
	// however healthy the aggregate throughput looks.
	WorstReadMs int
}

// DiskThroughput is one disk's whole curve: what it gives as readers pile on,
// and where (if anywhere) it falls over.
type DiskThroughput struct {
	// Device identifies the physical disk (the filesystem's device id). Two
	// libraries on the same drive share one entry: they also share its
	// throughput, and counting it twice would promise capacity that isn't there.
	Device uint64
	// SamplePath is one file that lives on this disk, so an operator can tell
	// which drive is being talked about.
	SamplePath string
	// Levels is the measured curve, narrowest width first.
	Levels []DiskLevel
	// BytesPerSec is the BEST this disk ever gave, at any width.
	BytesPerSec int64
	// BestReaders is the width that achieved it - which is, in effect, how many
	// concurrent streams this drive actually likes.
	BestReaders int
	// CollapsesAt is the width at which throughput fell to a fraction of its
	// best. Zero when it never collapsed (a healthy local disk just plateaus).
	// Non-zero is the single most actionable number in the whole report: past
	// this many concurrent readers, this storage gets DRAMATICALLY worse, not
	// merely no better.
	CollapsesAt int
}

// collapseRatio: below this share of its best, a disk has not plateaued - it has
// fallen over. A plateau (the same throughput at twice the readers) is normal
// and healthy; half the throughput at twice the readers is a storage path that
// serialises, and it will keep getting worse.
const collapseRatio = 0.5

// DiskProbe is the whole storage picture: what each disk gives, and what they
// give together.
type DiskProbe struct {
	// Disks is the per-drive breakdown, slowest first - the slowest is the one
	// that decides how bad it gets when viewers happen to pick films on it.
	Disks []DiskThroughput
	// TotalBytesPerSec is what the drives sustained ALL AT ONCE, at their best
	// width. Every disk is read simultaneously, so this already carries whatever
	// they contend for (a shared controller, one HBA, one network link).
	TotalBytesPerSec int64
	// CollapsesAtReaders is the narrowest width at which ANY disk fell over.
	// Zero when none did. When it is not zero it is, on a direct-play server, the
	// number that decides everything: it is roughly how many people can watch at
	// once before the storage - not the CPU, not Evey - makes playback crawl.
	CollapsesAtReaders int
	// Measured is false when no candidate file could be read at all (no path
	// known, permissions, a vanished file) - the report then omits the figure
	// rather than inventing one.
	Measured bool
}

// bestWidth is the width at which the drives, together, gave the most.
func (p DiskProbe) bestWidth() int {
	width := 0
	for _, d := range p.Disks {
		width = max(width, d.BestReaders)
	}
	return width
}

// atWidth is what the drives give TOGETHER when `readers` viewers are pulling at
// once - measured at the widest sampled level that does not exceed the audience,
// so the answer is a floor rather than a flattering extrapolation.
//
// This, and not TotalBytesPerSec, is the number an audience of that size is up
// against. The peak is measured at whatever width the disk happens to like (1 or
// 4 readers on spinning rust, sequential, readahead doing the work), and quoting
// it next to a 21-viewer workload invents headroom out of thin air: the operator
// reads "1151 of 2872 Mb/s" and concludes he is at 40% of his disk, when he is in
// fact at its knees. The gap between these two figures is not spare capacity, it
// is the price of the seeks.
// An audience of 21 spread over 2 disks does NOT put 21 readers on each drive:
// it puts about 11 on each. The load a single spindle feels is the audience
// DIVIDED by the drives it is spread across, which is the whole point of running
// a library on several disks - and reading the curve at the wrong width would
// invent a collapse as readily as it invents headroom.
func (p DiskProbe) atWidth(readers int) int64 {
	if readers <= 0 || len(p.Disks) == 0 {
		return 0
	}
	perDisk := (readers + len(p.Disks) - 1) / len(p.Disks)
	var total int64
	for _, d := range p.Disks {
		if len(d.Levels) == 0 {
			continue
		}
		// Levels are sorted narrowest first: walk to the widest sampled level
		// that the drive actually faces, and quote that. Never extrapolate past
		// what was measured.
		at := d.Levels[0].BytesPerSec
		for _, lvl := range d.Levels {
			if lvl.Readers > perDisk {
				break
			}
			at = lvl.BytesPerSec
		}
		total += at
	}
	return total
}

// measureDiskThroughput sweeps every disk the library spans, at several
// concurrency widths, and reports each drive's curve plus where it falls over.
func measureDiskThroughput(ctx context.Context, candidates []MediaCandidate, rng *rand.Rand) DiskProbe {
	byDisk := groupByDisk(candidates)
	if len(byDisk) == 0 {
		return DiskProbe{}
	}

	benchCtx, cancel := context.WithTimeout(ctx, diskBenchTimeout)
	defer cancel()

	curves := make(map[uint64]*DiskThroughput, len(byDisk))
	for device, paths := range byDisk {
		curves[device] = &DiskThroughput{Device: device, SamplePath: paths[0]}
	}

	// One width at a time, but EVERY DISK AT ONCE within it: that is how an
	// audience hits the array, and it is the only way to catch a bottleneck the
	// drives share (one controller, one HBA, one network link) that measuring
	// them in turn would miss.
	for _, readers := range diskBenchLevels {
		if benchCtx.Err() != nil {
			break
		}
		var wg sync.WaitGroup
		for device, paths := range byDisk {
			wg.Add(1)
			go func(device uint64, paths []string) {
				defer wg.Done()
				lvl := readDiskAtWidth(benchCtx, paths, readers, rng)
				if lvl.BytesPerSec > 0 {
					curves[device].Levels = append(curves[device].Levels, lvl)
				}
			}(device, paths)
		}
		wg.Wait()
	}

	out := DiskProbe{}
	for _, c := range curves {
		if len(c.Levels) == 0 {
			continue
		}
		sort.Slice(c.Levels, func(i, j int) bool { return c.Levels[i].Readers < c.Levels[j].Readers })
		summarise(c)
		out.Disks = append(out.Disks, *c)
		out.TotalBytesPerSec += c.BytesPerSec
		if c.CollapsesAt > 0 && (out.CollapsesAtReaders == 0 || c.CollapsesAt < out.CollapsesAtReaders) {
			out.CollapsesAtReaders = c.CollapsesAt
		}
	}
	if len(out.Disks) == 0 {
		return DiskProbe{}
	}
	// Slowest first: that drive decides how bad it gets when viewers happen to
	// pick films that live on it.
	sort.Slice(out.Disks, func(i, j int) bool { return out.Disks[i].BytesPerSec < out.Disks[j].BytesPerSec })
	out.Measured = true
	return out
}

// summarise reads a disk's curve: its best throughput, the width that achieved
// it, and the width (if any) at which it fell over rather than merely plateaued.
func summarise(c *DiskThroughput) {
	for _, lvl := range c.Levels {
		if lvl.BytesPerSec > c.BytesPerSec {
			c.BytesPerSec, c.BestReaders = lvl.BytesPerSec, lvl.Readers
		}
	}
	for _, lvl := range c.Levels {
		// Only ever a collapse if it happens AFTER the peak: a narrow width that
		// is slower than the peak is just a disk that likes some parallelism.
		if lvl.Readers > c.BestReaders && float64(lvl.BytesPerSec) < float64(c.BytesPerSec)*collapseRatio {
			c.CollapsesAt = lvl.Readers
			return
		}
	}
}

// readDiskAtWidth runs `readers` concurrent readers against one disk's files and
// reports what it delivered and the worst single read anyone saw.
func readDiskAtWidth(ctx context.Context, paths []string, readers int, rng *rand.Rand) DiskLevel {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		total int64
		worst time.Duration
	)
	started := time.Now()
	for i := range readers {
		wg.Add(1)
		// Each reader starts somewhere else in its own file: readahead can't
		// serve them all, and neither can a cache holding the first few MiB.
		path := paths[i%len(paths)]
		offset := rng.Int64N(1 << 30)
		go func(path string, offset int64) {
			defer wg.Done()
			n, slowest := readChunks(ctx, path, offset)
			mu.Lock()
			total += n
			worst = max(worst, slowest)
			mu.Unlock()
		}(path, offset)
	}
	wg.Wait()

	elapsed := time.Since(started).Seconds()
	if total == 0 || elapsed <= 0 {
		return DiskLevel{}
	}
	return DiskLevel{
		Readers:     readers,
		BytesPerSec: int64(float64(total) / elapsed),
		WorstReadMs: int(worst.Milliseconds()),
	}
}

// diskOf reports which physical drive a file lives on. Not knowing (a vanished
// file, a path the server can't stat) is not an error - the caller simply can't
// balance or measure that one.
func diskOf(path string) (uint64, bool) {
	if path == "" {
		return 0, false
	}
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, false
	}
	return st.Dev, true
}

// groupByDisk buckets candidate files by the physical drive they live on, so a
// library spread across several disks is measured as several disks. Files whose
// device can't be determined are skipped rather than lumped together - guessing
// would merge drives that don't share throughput.
func groupByDisk(candidates []MediaCandidate) map[uint64][]string {
	byDisk := make(map[uint64][]string)
	for _, c := range candidates {
		dev, ok := diskOf(c.Path)
		if !ok {
			continue
		}
		if len(byDisk[dev]) >= diskBenchReadersPerDisk {
			continue
		}
		if _, known := byDisk[dev]; !known && len(byDisk) >= diskBenchMaxDisks {
			continue
		}
		byDisk[dev] = append(byDisk[dev], c.Path)
	}
	return byDisk
}

// readChunks pulls diskBenchBytesPerReader from path, starting at offset (or at
// the top when the file is shorter). It returns how many bytes it got and the
// slowest single chunk - the latter is what a viewer actually feels, and it can
// climb into seconds on storage that serialises even while the aggregate figure
// still looks respectable. A file that cannot be opened contributes nothing
// rather than failing the probe.
func readChunks(ctx context.Context, path string, offset int64) (int64, time.Duration) {
	f, err := os.Open(path) //nolint:gosec // an operator-configured library path the server already streams from
	if err != nil {
		return 0, 0
	}
	defer func() { _ = f.Close() }()

	if info, err := f.Stat(); err == nil {
		if size := info.Size(); size > diskBenchBytesPerReader {
			offset %= size - diskBenchBytesPerReader
		} else {
			offset = 0
		}
	}

	buf := make([]byte, diskBenchChunk)
	var read int64
	var worst time.Duration
	for read < diskBenchBytesPerReader {
		if ctx.Err() != nil {
			break
		}
		started := time.Now()
		n, err := f.ReadAt(buf, offset+read)
		worst = max(worst, time.Since(started))
		read += int64(n)
		if err != nil {
			break // EOF or a short file: whatever we got still counts
		}
	}
	return read, worst
}

// slowest is what the weakest drive delivered - the one that decides how bad it
// gets when viewers happen to pick films that live on it.
func (p DiskProbe) slowest() int64 {
	if len(p.Disks) == 0 {
		return 0
	}
	return p.Disks[0].BytesPerSec // sorted slowest-first
}
