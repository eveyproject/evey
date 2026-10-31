package filesystem

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// mountinfo.go - which filesystem backs a given path, and whether it is a
// network share.
//
// This exists for one reason: the watcher's inotify liveness probe cannot tell
// the truth on a network mount. The probe writes a file and waits for its own
// event - but on NFS/SMB the write goes through the LOCAL kernel, which happily
// raises the event, so the probe passes. Meanwhile a write made by any OTHER
// client on that share produces no inotify event at all, because inotify is a
// kernel-local mechanism with no network protocol behind it.
//
// A green probe on a network share is therefore a false green, and acting on it
// (granting the library the spaced-out backstop cadence) would leave media
// added from another machine undiscovered for up to an hour. Detecting the
// filesystem type lets the watcher decline to vouch for those libraries instead
// of trusting a probe that cannot fail.

// networkFSTypes are the filesystem types whose contents can change without
// the local kernel knowing. fuse.sshfs and the *fs spellings are included
// because they show up on real NAS setups.
var networkFSTypes = map[string]bool{
	"nfs":         true,
	"nfs4":        true,
	"cifs":        true,
	"smb3":        true,
	"smbfs":       true,
	"afs":         true,
	"9p":          true, // WSL2 / VM shares
	"fuse.sshfs":  true,
	"fuse.rclone": true,
	"glusterfs":   true,
	"ceph":        true,
	// Union and shared-folder filesystems fed by another host or the VM host:
	// writes made outside the guest raise no local event.
	"fuse.mergerfs": true,
	"fuse.shfs":     true,
	"virtiofs":      true,
	"fuse.grpcfuse": true, // Docker Desktop's share
}

// mountEntry is one line of /proc/mounts, reduced to what we need.
type mountEntry struct {
	point  string
	fsType string
}

// parseMounts reads /proc/mounts-formatted content. Field order is
// "device mountpoint fstype options ...", space-separated, with octal escapes
// in the mount point (a path containing a space is written as \040).
func parseMounts(r io.Reader) []mountEntry {
	var out []mountEntry
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		out = append(out, mountEntry{
			point:  unescapeMountPath(fields[1]),
			fsType: fields[2],
		})
	}
	return out
}

// unescapeMountPath decodes the octal escapes the kernel uses for characters
// that would otherwise break the space-separated format. Without this a library
// under a path containing a space would never match its mount entry, and the
// caller would silently fall back to "not a network mount".
func unescapeMountPath(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			var v int
			ok := true
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

// fsTypeFor returns the filesystem type backing path, by longest-prefix match
// over the mount table. Longest wins because mount points nest: /media/nas is
// a more specific answer than / for a file under it.
func fsTypeFor(entries []mountEntry, path string) (string, bool) {
	clean := filepath.Clean(path)
	best := ""
	bestType := ""
	for _, e := range entries {
		if !pathUnder(clean, e.point) {
			continue
		}
		if len(e.point) > len(best) {
			best, bestType = e.point, e.fsType
		}
	}
	return bestType, bestType != ""
}

// pathUnder reports whether path is mountPoint or lives beneath it. Compared
// segment-wise so /media/nas-backup is not treated as being under /media/nas.
func pathUnder(path, mountPoint string) bool {
	mp := filepath.Clean(mountPoint)
	if path == mp {
		return true
	}
	if mp == string(filepath.Separator) {
		return true
	}
	return strings.HasPrefix(path, mp+string(filepath.Separator))
}

// IsNetworkMount reports whether path is backed by a filesystem whose contents
// can change without the local kernel raising an inotify event, along with the
// detected type for logging.
//
// Fails OPEN: if the mount table cannot be read (a non-Linux host, a locked-down
// container), it reports "not a network mount" and the caller keeps its previous
// behaviour. A false negative costs a stale library for up to the backstop
// interval; a false positive would pin a perfectly local library to permanent
// frequent scanning, which is the more expensive mistake to make blindly.
func IsNetworkMount(path string) (fsType string, isNetwork bool) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	t, ok := fsTypeFor(parseMounts(f), path)
	if !ok {
		return "", false
	}
	return t, networkFSTypes[t]
}
