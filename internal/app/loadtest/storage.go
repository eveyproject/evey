package loadtest

import (
	"sort"
	"strings"
)

// libraryStorage names the filesystem types the candidate files live on and
// whether any of them is OPAQUE to this kernel's accounting. The report carries
// both, up front and whatever the outcome, because two of its instruments go
// blind on such a mount and a reader who does not know that reads the wrong
// verdict:
//
//   - the disk counter (cgroup io.stat) only sees block devices this kernel
//     drives. A FUSE pool (mergerfs, rclone, sshfs) reads in another process,
//     a network share (NFS, CIFS) or a VM share (9p, virtiofs) on another kernel
//     entirely: the counter sits at zero however hard the disks work;
//   - cold-cache eviction (posix_fadvise DONTNEED) drops the pages of the file
//     it is given - the mount's own view. The disks underneath a FUSE pool keep
//     their pages in this very kernel under other inodes, so "cold" is only
//     partly cold there; on a network share the cache that matters is the
//     server's.
//
// A pool also collapses every disk behind it into one device, so the disk probe
// counts one disk where there are three. None of this is a bug to fix here: it
// is what the mount is. The report says so instead of letting a number imply
// what it cannot know.
func libraryStorage(candidates []MediaCandidate, fsTypeOf func(path string) string) (fsTypes string, opaque bool) {
	seen := map[string]bool{}
	for _, c := range candidates {
		if c.Path == "" {
			continue
		}
		t := fsTypeOf(c.Path)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
	}
	types := make([]string, 0, len(seen))
	for t := range seen {
		types = append(types, t)
		if storageOpaque(t) {
			opaque = true
		}
	}
	sort.Strings(types)
	return strings.Join(types, ", "), opaque
}

// storageOpaque reports whether the kernel's block-device accounting and page
// cache cannot see through this filesystem type to the disks.
func storageOpaque(fsType string) bool {
	if strings.HasPrefix(fsType, "fuse") {
		return true
	}
	switch fsType {
	case "nfs", "nfs4", "cifs", "smb3", "smbfs", "afs", "9p", "virtiofs", "glusterfs", "ceph":
		return true
	}
	return false
}
