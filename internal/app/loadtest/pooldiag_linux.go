//go:build linux

package loadtest

import (
	"strings"

	"golang.org/x/sys/unix"
)

// mergerfsBranchOf asks a mergerfs mount which branch really holds a file. The
// xattr is answered by mergerfs itself, so it works for any user who can open
// the file; every other filesystem answers ENOTSUP/ENODATA and the branch is
// simply unknown.
func mergerfsBranchOf(path string) string {
	buf := make([]byte, 4096)
	n, err := unix.Getxattr(path, "user.mergerfs.basepath", buf)
	if err != nil || n <= 0 {
		return ""
	}
	return strings.TrimRight(string(buf[:n]), "\x00")
}
