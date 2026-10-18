package sysinfo

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DiskIO is the activity of one physical disk over the sampling window. The
// counters are the whole machine's (/proc/diskstats sees every process, a
// neighbour container or a file copy included), so the UI labels them as the
// machine's, never as Evey's.
type DiskIO struct {
	// Device is the kernel name (sda). It can change between two boots, so
	// it is shown but never stored as the disk's identity.
	Device string
	// ID is stable across boots: the wwid, else the serial number, else the
	// model. In a VM every disk reports the same model, which is why the
	// model comes last.
	ID string
	// Label is what the admin Reads: the model plus the tail of the ID that
	// tells two identical disks apart.
	Label      string
	Rotational bool
	// UtilPercent is the share of the window the disk had at least one
	// request in flight. A busy SSD or a parallel array reads 100 here
	// without being saturated, so it corroborates AwaitMs, nothing more.
	UtilPercent float64
	// AwaitMs is the mean time a READ waited and was served (ReadAwaitMs), the
	// wait the load test judges on too. Compared with the disk's own quiet
	// value it is the one signal that separated a load test that held from one
	// that broke. Before 2026-10-18 it averaged reads and writes together.
	AwaitMs      float64
	ReadBps      int64
	WriteBps     int64
	RequestsPerS float64
}

// NetIO is the traffic of every non-loopback interface the process can see,
// summed over the sampling window.
type NetIO struct {
	RxBps int64
	TxBps int64
}

// Disk saturation thresholds, the ONE copy the dashboard and the load test's
// verdict (internal/app/loadtest/verdict.go) both apply: a disk is saturated
// when its mean read wait is at least SaturatedAwaitFloorMs and at least
// SaturatedAwaitRatio times its quiet wait, with enough activity or iowait to
// corroborate.
const (
	IdleAwaitFloorMs      = 4.0
	SaturatedAwaitFloorMs = 20.0
	SaturatedAwaitRatio   = 2.5
	SaturatedUtilMinPct   = 30.0
	SaturatedIOWaitMinPct = 25.0
	// IdleAwaitUtilCeilPct is the busy share under which a sample's wait
	// stands for the disk at rest.
	IdleAwaitUtilCeilPct   = 10.0
	diskstatsSectorBytes   = 512
	diskIDTailLen          = 12
	diskstatsMinFieldCount = 14
)

// SaturationAwaitMs is the wait above which a disk counts as saturated, given
// its quiet wait: a fixed floor and a multiple of the quiet value, whichever
// is higher. In a VM the guest's wait includes the host's queue, so the
// absolute numbers inflate and the ratio carries the decision.
func SaturationAwaitMs(idleAwaitMs float64) float64 {
	return max(SaturatedAwaitFloorMs, SaturatedAwaitRatio*max(idleAwaitMs, IdleAwaitFloorMs))
}

// DiskSaturated applies the thresholds to one window: the wait is above the
// saturation line and either the disk was busy enough or the CPUs were
// waiting on it.
func DiskSaturated(awaitMs, idleAwaitMs, utilPct, iowaitPct float64) bool {
	if awaitMs < SaturationAwaitMs(idleAwaitMs) {
		return false
	}
	return utilPct >= SaturatedUtilMinPct || iowaitPct >= SaturatedIOWaitMinPct
}

// IdleAwaitCandidate reports whether a sample is quiet enough to stand for the
// disk's wait at rest: it served requests, but was busy for a small share of
// the window.
func IdleAwaitCandidate(awaitMs, utilPct float64) bool {
	return awaitMs > 0 && utilPct < IdleAwaitUtilCeilPct
}

// DiskCounters is one /proc/diskstats line, cumulative since boot. The one
// reading of that file for the admin dashboard and the load test alike: they
// used to parse it twice, and judged saturation on different waits (reads and
// writes together here, reads alone in the bench), so the dashboard's
// "saturated" badge could contradict the bench verdict on the same disk.
type DiskCounters struct {
	Reads, SectorsRead, ReadMs      uint64
	Writes, SectorsWritten, WriteMs uint64
	IOTicksMs                       uint64
	// WeightedMs is the time every request in flight spent waiting, summed:
	// over an interval it is the mean queue depth (iostat's aqu-sz).
	WeightedMs uint64
}

// ReadAwaitMs is the mean time one read waited and was served between two
// readings, 0 when no read completed. Reads alone, on purpose: a viewer waits
// on a read, and a read queued behind writes already counts that queueing.
// This is the wait both the dashboard and the load test judge saturation on.
func ReadAwaitMs(prev, cur DiskCounters) float64 {
	if cur.Reads <= prev.Reads || cur.ReadMs < prev.ReadMs {
		return 0
	}
	return float64(cur.ReadMs-prev.ReadMs) / float64(cur.Reads-prev.Reads)
}

type diskSnapshot struct {
	at    time.Time
	disks map[string]DiskCounters
	rx    uint64
	tx    uint64
	ok    bool
}

// ioPrev is the previous disk and network reading of the cached Collect,
// guarded by collectMu like cpuPrev: every window spans the whole gap since
// the previous sample. A Sampler keeps its own.
var ioPrev diskSnapshot

// Paths read by the samplers; variables so tests point them at fixtures.
var (
	procDiskstatsPath = "/proc/diskstats"
	procNetDevPath    = "/proc/net/dev"
	sysBlockPath      = "/sys/block"
)

// primeIO takes the first reading when none exists, so the first Collect can
// report a window at all (the CPU sampler sleeps 500 ms right after).
func primeIO(prev *diskSnapshot) {
	if !prev.ok {
		*prev = readIOSnapshot()
	}
}

// sampleIO diffs the current counters against the previous reading and
// advances it. Without a previous reading it returns nothing rather than
// since-boot averages, which would read as a flat lie.
func sampleIO(state *diskSnapshot) ([]DiskIO, NetIO) {
	cur := readIOSnapshot()
	prev := *state
	*state = cur
	if !cur.ok || !prev.ok {
		return nil, NetIO{}
	}
	elapsed := cur.at.Sub(prev.at).Seconds()
	if elapsed <= 0 {
		return nil, NetIO{}
	}
	var disks []DiskIO
	for _, name := range sortedKeys(cur.disks) {
		c := cur.disks[name]
		p, had := prev.disks[name]
		if !had {
			continue
		}
		disks = append(disks, diffDisk(name, p, c, elapsed))
	}
	var net NetIO
	if cur.rx >= prev.rx {
		net.RxBps = int64(float64(cur.rx-prev.rx) * 8 / elapsed)
	}
	if cur.tx >= prev.tx {
		net.TxBps = int64(float64(cur.tx-prev.tx) * 8 / elapsed)
	}
	return disks, net
}

func diffDisk(name string, p, c DiskCounters, elapsed float64) DiskIO {
	d := DiskIO{Device: name}
	d.ID, d.Label = diskIdentity(name)
	d.Rotational = readSysFlag(filepath.Join(sysBlockPath, name, "queue", "rotational"))
	reads := c.Reads - min(c.Reads, p.Reads)
	writes := c.Writes - min(c.Writes, p.Writes)
	if n := reads + writes; n > 0 {
		d.RequestsPerS = float64(n) / elapsed
	}
	d.AwaitMs = ReadAwaitMs(p, c)
	d.UtilPercent = min(100, float64(c.IOTicksMs-min(c.IOTicksMs, p.IOTicksMs))/(elapsed*1000)*100)
	d.ReadBps = int64(float64((c.SectorsRead-min(c.SectorsRead, p.SectorsRead))*diskstatsSectorBytes) / elapsed)
	d.WriteBps = int64(float64((c.SectorsWritten-min(c.SectorsWritten, p.SectorsWritten))*diskstatsSectorBytes) / elapsed)
	return d
}

func readIOSnapshot() diskSnapshot {
	s := diskSnapshot{at: time.Now(), disks: readDiskstats()}
	s.rx, s.tx = readNetDev()
	s.ok = s.disks != nil
	return s
}

// readDiskstats returns the counters of every whole physical disk: a name
// listed under /sys/block (partitions are not), with no slaves (a dm, md or
// LVM volume is made of disks that are listed on their own) and not one of
// the pseudo devices. nil when the file cannot be read.
func readDiskstats() map[string]DiskCounters {
	f, err := os.Open(procDiskstatsPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	return ParseDiskstats(f, isWholePhysicalDisk)
}

// ParseDiskstats reads /proc/diskstats content, keeping the devices keep
// accepts (each caller has its own idea of a disk: the dashboard drops the
// members of an array, the bench keeps the array).
func ParseDiskstats(r io.Reader, keep func(name string) bool) map[string]DiskCounters {
	out := map[string]DiskCounters{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < diskstatsMinFieldCount {
			continue
		}
		name := fields[2]
		if !keep(name) {
			continue
		}
		u := func(i int) uint64 {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			return v
		}
		// 3 reads, 4 merged, 5 sectors read, 6 read ms, 7 writes, 8 merged,
		// 9 sectors written, 10 write ms, 11 in flight, 12 io ticks ms,
		// 13 weighted ms.
		out[name] = DiskCounters{
			Reads: u(3), SectorsRead: u(5), ReadMs: u(6),
			Writes: u(7), SectorsWritten: u(9), WriteMs: u(10),
			IOTicksMs: u(12), WeightedMs: u(13),
		}
	}
	return out
}

var pseudoDiskPrefixes = []string{"loop", "ram", "zram", "sr", "fd", "nbd", "dm-", "md"}

func isWholePhysicalDisk(name string) bool {
	for _, p := range pseudoDiskPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	dir := filepath.Join(sysBlockPath, name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return false
	}
	slaves, _ := os.ReadDir(filepath.Join(dir, "slaves"))
	return len(slaves) == 0
}

// diskIdentity picks the most stable identifier sysfs offers and builds the
// label the admin sees. The device letter is never part of the ID: it moves
// between boots (the production VM's sda and sdb swapped on 2026-10-06).
func diskIdentity(name string) (id, label string) {
	dev := filepath.Join(sysBlockPath, name, "device")
	model := readSysString(filepath.Join(dev, "model"))
	for _, p := range []string{
		filepath.Join(dev, "wwid"),
		filepath.Join(sysBlockPath, name, "wwid"),
		filepath.Join(dev, "serial"),
	} {
		if v := readSysString(p); v != "" {
			id = v
			break
		}
	}
	switch {
	case id == "" && model == "":
		return name, name
	case id == "":
		return model, model
	case model == "":
		return id, idTail(id)
	}
	return id, model + " " + idTail(id)
}

// idTail keeps the part of an ID that distinguishes it: the last token of a
// spaced wwid ("drive-scsi1" in a QEMU one), or the last characters of a
// compact one ("...552efcdf0c" in a naa one).
func idTail(id string) string {
	fields := strings.Fields(id)
	last := fields[len(fields)-1]
	if len(last) > diskIDTailLen {
		last = last[len(last)-diskIDTailLen:]
	}
	return last
}

func readSysString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

func readSysFlag(path string) bool {
	return readSysString(path) == "1"
}

// readNetDev sums the bytes received and sent on every interface but the
// loopback. Inside a container these are the container's interfaces: what
// crossed into and out of Evey's network namespace.
func readNetDev() (rx, tx uint64) {
	f, err := os.Open(procNetDevPath)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || name == "" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(fields[0], 10, 64)
		t, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx
}

func sortedKeys(m map[string]DiskCounters) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
