//go:build linux

package loadtest

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// devices_linux.go - which block device each library location is read from.
//
// Device letters are not stable: they rotate at reboot, and a report that says
// "sdc" without saying what sdc held that day cannot be read a week later. The
// run therefore resolves the mapping ONCE at its start and prints it, and the
// disk figures it reports are summed over exactly these devices.
//
// Two ways to resolve a location, tried in order:
//
//   - stat: a file's st_dev names its filesystem's device; /sys/dev/block
//     turns that into the whole disk (a partition's parent) or the volume (an
//     LVM or md device). This is the plain case - ext4/xfs on a disk or a
//     volume.
//   - a read: on a mount whose st_dev is anonymous (a FUSE pool, btrfs, which
//     gives every filesystem its own anonymous device, a network share), the
//     run reads a few MiB cold from a file on the location and watches which
//     device moved sectors. The device that moved most of those bytes is the
//     location's, the top of a stack when several moved them all (an md array
//     and the volume on it). Nothing on a network share moves a local disk,
//     and the location then stays unresolved - which is the truth: that disk
//     is not on this machine.
//
// A volume is then walked down to the physical disks under it (dm-0 -> md2 ->
// sda5, sdb5 -> sda, sdb, the Synology SHR stack): the queue that decides the
// verdict forms on the disks, and an md array's own counters carry no wait
// time on older kernels - judged on the array, a saturated pool reads as idle.

// DeviceMapping says which block device a library location is read from.
type DeviceMapping struct {
	// Location is the branch of a pooled mount, or the mount point, the titles
	// sit on - what an operator recognises.
	Location string
	// Device is the whole block device behind it ("sda", "nvme0n1", "dm-0").
	// Empty when no device on this machine could be tied to the location.
	Device string
	// Resolved says how: "stat" (the filesystem's device id), "read" (a cold
	// read moved its sectors), "" when unresolved.
	Resolved string
	// Disks are the physical disks under Device: Device itself for a plain
	// disk, the members of a volume (an md array, an LVM volume, both
	// stacked). What the run sums and judges.
	Disks []string
}

// deviceProbeBytes is how much a read-probe pulls from one file of a branch:
// enough that the disk behind it moves unmistakably more sectors than any
// background reader does in the same second, small enough to cost nothing.
const deviceProbeBytes = 16 << 20

// deviceProbeMinShare is the share of the probe's bytes the winning disk must
// have moved. Below it the read came from a cache (the branch's own page
// cache, a controller) and no disk can honestly be named.
const deviceProbeMinShare = 0.5

// libraryDevices resolves the block device behind every location the
// candidates sit on. branchOf resolves a file to its pool branch ("" when the
// mount is not a pool); fsRoot roots /proc and /sys for tests.
func libraryDevices(ctx context.Context, candidates []MediaCandidate, branchOf func(string) string, fsRoot string, rng *rand.Rand) []DeviceMapping {
	// Group the candidates by location: one probe per branch, not per title.
	byLocation := map[string][]string{}
	var order []string
	for _, c := range candidates {
		if c.Path == "" {
			continue
		}
		loc := ""
		if branchOf != nil {
			loc = branchOf(c.Path)
		}
		if loc == "" {
			loc = mountPointOf(fsRoot, c.Path)
		}
		if loc == "" {
			continue
		}
		if _, seen := byLocation[loc]; !seen {
			order = append(order, loc)
		}
		byLocation[loc] = append(byLocation[loc], c.Path)
	}
	out := make([]DeviceMapping, 0, len(order))
	for _, loc := range order {
		m := DeviceMapping{Location: loc}
		if dev := deviceOfPath(fsRoot, byLocation[loc][0]); dev != "" {
			m.Device, m.Resolved = dev, "stat"
		} else if dev := deviceByRead(ctx, fsRoot, byLocation[loc], rng); dev != "" {
			m.Device, m.Resolved = dev, "read"
		}
		if m.Device != "" {
			m.Disks = physicalDisks(fsRoot, m.Device)
		}
		out = append(out, m)
	}
	return out
}

// mappedDevices is the set of physical disks the mapping named,
// deduplicated, in mapping order.
func mappedDevices(mapping []DeviceMapping) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range mapping {
		disks := m.Disks
		if len(disks) == 0 && m.Device != "" {
			disks = []string{m.Device}
		}
		for _, d := range disks {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// allMapped reports whether every library location was tied to a disk of
// this machine. A location left unresolved (a network share, a volume the
// container cannot see) is storage the run could not measure.
func allMapped(mapping []DeviceMapping) bool {
	if len(mapping) == 0 {
		return false
	}
	for _, m := range mapping {
		if m.Device == "" {
			return false
		}
	}
	return true
}

// physicalDisks walks a device down to the physical disks under it: a
// partition to its disk, a volume (dm, md) through its slaves, recursively.
// A device with no slaves is its own disk.
func physicalDisks(fsRoot, name string) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(n string, depth int)
	walk = func(n string, depth int) {
		disk := diskOfName(fsRoot, n)
		if disk == "" || depth > 8 {
			return
		}
		slaves, _ := os.ReadDir(filepath.Join(fsRoot, "sys/block", disk, "slaves"))
		if len(slaves) == 0 {
			if !seen[disk] {
				seen[disk] = true
				out = append(out, disk)
			}
			return
		}
		for _, s := range slaves {
			walk(s.Name(), depth+1)
		}
	}
	walk(name, 0)
	sort.Strings(out)
	return out
}

// diskOfName is the entry of /sys/block a device name belongs to: itself for
// a disk or a volume, the parent disk for a partition; "" when unknown.
func diskOfName(fsRoot, name string) string {
	if _, err := os.Stat(filepath.Join(fsRoot, "sys/block", name)); err == nil {
		return name
	}
	target, err := os.Readlink(filepath.Join(fsRoot, "sys/class/block", name))
	if err != nil {
		return ""
	}
	parent := filepath.Base(filepath.Dir(target))
	if _, err := os.Stat(filepath.Join(fsRoot, "sys/block", parent)); err != nil {
		return ""
	}
	return parent
}

// deviceOfPath is the whole block device a file's filesystem sits on, by its
// st_dev, or "" when the device id names nothing under /sys/dev/block (a FUSE
// or network mount: an anonymous device).
func deviceOfPath(fsRoot, path string) string {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return ""
	}
	return wholeDiskOf(fsRoot, uint64(st.Dev)) //nolint:unconvert // Dev is uint64 on amd64 and int32 on some arm ABIs
}

// wholeDiskOf resolves a device id to the whole disk the kernel accounts in
// /proc/diskstats: a partition (sda1) to its parent (sda), a volume (dm-0) to
// itself.
func wholeDiskOf(fsRoot string, dev uint64) string {
	major := (dev>>8)&0xfff | (dev>>32)&^0xfff
	minor := dev&0xff | (dev>>12)&^0xff
	link := filepath.Join(fsRoot, "sys/dev/block", strconv.FormatUint(major, 10)+":"+strconv.FormatUint(minor, 10))
	target, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(target, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	name, parent := parts[len(parts)-1], parts[len(parts)-2]
	if parent != "block" {
		// .../block/<disk>/<partition>: the disk is the parent.
		name = parent
	}
	if _, err := os.Stat(filepath.Join(fsRoot, "sys/block", name)); err != nil {
		return ""
	}
	return name
}

// deviceByRead names the whole disk that moved the bytes of a cold read from
// one of the paths. The read is scattered: a random offset in a random file,
// so a page the previous run or the disk probe left warm is unlikely to answer
// it. The disk that moved at least deviceProbeMinShare of the bytes wins;
// none, and the location stays unresolved.
func deviceByRead(ctx context.Context, fsRoot string, paths []string, rng *rand.Rand) string {
	if len(paths) == 0 || rng == nil {
		return ""
	}
	path := paths[rng.IntN(len(paths))]
	f, err := os.Open(path) //nolint:gosec // a library file the server already streams
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil || fi.Size() < 2*deviceProbeBytes {
		return ""
	}
	// Drop what this kernel holds of the file first (on a pool only the mount's
	// own pages; the branch's are the cache the probe may still hit, which the
	// share threshold below catches). A refused eviction is the same case.
	if err := evictFromCache(path); err != nil {
		slog.Debug("loadtest: device probe could not evict the file first", slog.String("path", path), slog.Any("err", err))
	}
	off := rng.Int64N(fi.Size() - deviceProbeBytes)
	before := readDiskCounters(fsRoot)
	buf := make([]byte, 1<<20)
	var got int64
	deadline := time.Now().Add(10 * time.Second)
	for got < deviceProbeBytes && ctx.Err() == nil && time.Now().Before(deadline) {
		n, rerr := f.ReadAt(buf, off+got)
		got += int64(n)
		if rerr != nil {
			break
		}
	}
	after := readDiskCounters(fsRoot)
	if got <= 0 {
		return ""
	}
	// Every device that moved most of the read: one disk, or a stack (the
	// volume, the array under it, a mirror member) that all carried it.
	var movers []string
	for dev, c := range after {
		p, ok := before[dev]
		if !ok || c.sectorsRead < p.sectorsRead {
			continue
		}
		if moved := int64(c.sectorsRead-p.sectorsRead) * 512; float64(moved) >= float64(got)*deviceProbeMinShare {
			movers = append(movers, dev)
		}
	}
	return stackTop(fsRoot, movers)
}

// stackTop picks, among devices that all carried a read, the one no other of
// them sits on: the volume rather than its array, the array rather than a
// member. Its physical disks then cover every member, not only the one a
// mirror happened to read from. "" when none moved.
func stackTop(fsRoot string, movers []string) string {
	sort.Strings(movers)
	under := map[string]bool{}
	for _, m := range movers {
		for _, d := range physicalDisks(fsRoot, m) {
			if d != m {
				under[d] = true
			}
		}
		slaves, _ := os.ReadDir(filepath.Join(fsRoot, "sys/block", m, "slaves"))
		for _, s := range slaves {
			under[diskOfName(fsRoot, s.Name())] = true
		}
	}
	best, bestDepth := "", -1
	for _, m := range movers {
		if under[m] {
			continue
		}
		if d := stackDepth(fsRoot, m, 0); d > bestDepth {
			best, bestDepth = m, d
		}
	}
	return best
}

// stackDepth is how many volume levels sit under a device (0 for a disk).
func stackDepth(fsRoot, name string, depth int) int {
	if depth > 8 {
		return depth
	}
	slaves, _ := os.ReadDir(filepath.Join(fsRoot, "sys/block", diskOfName(fsRoot, name), "slaves"))
	deepest := depth
	for _, s := range slaves {
		deepest = max(deepest, stackDepth(fsRoot, s.Name(), depth+1))
	}
	return deepest
}

// mountPointOf is the longest mount point under which path sits, read from
// fsRoot/proc/self/mounts; "" when none matches.
func mountPointOf(fsRoot, path string) string {
	raw, err := os.ReadFile(filepath.Join(fsRoot, "proc/self/mounts"))
	if err != nil {
		return ""
	}
	best := ""
	for line := range strings.Lines(string(raw)) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mp := unescapeMount(fields[1])
		if (path == mp || strings.HasPrefix(path, strings.TrimRight(mp, "/")+"/")) && len(mp) > len(best) {
			best = mp
		}
	}
	return best
}

// unescapeMount decodes the octal escapes /proc/self/mounts uses for spaces
// and the like in a mount point.
func unescapeMount(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
