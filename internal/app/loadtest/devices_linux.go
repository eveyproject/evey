//go:build linux

package loadtest

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
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
//     turns that into the whole disk (a partition's parent). This is the plain
//     case - a library on ext4/xfs/btrfs, an LVM or md volume (the volume is
//     the device the kernel accounts, and that is the right one to sum).
//   - a read: on a mount this kernel cannot see through (a FUSE pool, where
//     st_dev is the pool's own anonymous device and the branches may not even
//     be visible from inside a container), the run reads a few MiB cold from a
//     file on each branch and watches which whole disk moved sectors. The disk
//     that moved most of those bytes is the branch's. Nothing on a network
//     share moves a local disk, and the branch then stays unresolved - which
//     is the truth: that disk is not on this machine.

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
		out = append(out, m)
	}
	return out
}

// mappedDevices is the set of whole disks the mapping named, deduplicated,
// in mapping order.
func mappedDevices(mapping []DeviceMapping) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range mapping {
		if m.Device != "" && !seen[m.Device] {
			seen[m.Device] = true
			out = append(out, m.Device)
		}
	}
	return out
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
	best, bestBytes := "", int64(0)
	for dev, c := range after {
		p, ok := before[dev]
		if !ok || c.sectorsRead < p.sectorsRead {
			continue
		}
		if moved := int64(c.sectorsRead-p.sectorsRead) * 512; moved > bestBytes {
			best, bestBytes = dev, moved
		}
	}
	if float64(bestBytes) < float64(got)*deviceProbeMinShare {
		return ""
	}
	return best
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
