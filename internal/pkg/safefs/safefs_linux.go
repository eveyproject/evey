//go:build linux

package safefs

import (
	"errors"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// statOpenFlags: O_PATH needs no read permission and never opens the target
// (a FIFO is not blocked on, a device not touched).
const statOpenFlags = unix.O_PATH

// noOpenat2 latches once the kernel or seccomp refuses openat2; tests force it
// to exercise the walk fallback.
var noOpenat2 atomic.Bool

// openat2Beneath reports ok=false when openat2 is unavailable, so the caller
// falls back to the component walk.
func openat2Beneath(rootfd int, rel string, flags int) (int, bool, error) {
	if noOpenat2.Load() {
		return -1, false, nil
	}
	how := unix.OpenHow{
		Flags:   uint64(flags | unix.O_NOFOLLOW | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	}
	fd, err := unix.Openat2(rootfd, rel, &how)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EPERM) {
		noOpenat2.Store(true)
		return -1, false, nil
	}
	return fd, true, err
}
