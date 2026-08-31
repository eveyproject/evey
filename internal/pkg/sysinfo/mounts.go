package sysinfo

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MountFSType returns the filesystem type of the mount backing path - "ext4",
// "fuse.mergerfs", "nfs4", "9p", "virtiofs"... - by longest-prefix match over
// /proc/self/mounts. Empty when the mount table cannot be read (a non-Linux
// host) or when nothing matches.
//
// The type is what tells a measurement whether the kernel can even see the
// disks: a FUSE pool, a network share or a VM share does its reads in another
// process, another kernel or another machine, so the block counters stay at
// zero and a page-cache eviction stops at the mount.
func MountFSType(path string) string {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	return mountFSType(parseMountTable(f), path)
}

// mountEntry is one line of /proc/mounts, reduced to what MountFSType needs.
type mountEntry struct {
	point  string
	fsType string
}

// parseMountTable reads /proc/mounts-formatted content: "device mountpoint
// fstype options ...", space-separated, with the kernel's octal escapes in the
// mount point (a path containing a space is written as \040).
func parseMountTable(r io.Reader) []mountEntry {
	var out []mountEntry
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		out = append(out, mountEntry{point: unescapeMountPath(fields[1]), fsType: fields[2]})
	}
	return out
}

// unescapeMountPath decodes the octal escapes the kernel uses for characters
// that would break the space-separated format.
func unescapeMountPath(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			v, ok := 0, true
			for _, c := range s[i+1 : i+4] {
				if c < '0' || c > '7' {
					ok = false
					break
				}
				v = v*8 + int(c-'0')
			}
			if ok {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// mountFSType picks the longest mount point that contains path: mounts nest,
// and /media/pool is a more specific answer than / for a file under it.
func mountFSType(entries []mountEntry, path string) string {
	clean := filepath.Clean(path)
	best, bestType := "", ""
	for _, e := range entries {
		if !pathUnderMount(clean, e.point) {
			continue
		}
		if len(e.point) > len(best) {
			best, bestType = e.point, e.fsType
		}
	}
	return bestType
}

// pathUnderMount reports whether path is mountPoint or lives beneath it,
// compared segment-wise so /media/pool-backup is not under /media/pool.
func pathUnderMount(path, mountPoint string) bool {
	mp := filepath.Clean(mountPoint)
	if path == mp || mp == string(filepath.Separator) {
		return true
	}
	return strings.HasPrefix(path, mp+string(filepath.Separator))
}
