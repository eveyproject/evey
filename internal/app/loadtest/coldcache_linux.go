//go:build linux

package loadtest

import (
	"os"

	"golang.org/x/sys/unix"
)

// evictFromCache asks the kernel to drop this file's pages from the page cache
// (POSIX_FADV_DONTNEED). Clean read-only pages are freed on the spot, no
// privileges needed, so the next read has to come from the storage. Pages the
// server holds in flight stay put - which is why cold-cache mode re-evicts
// periodically rather than once: a single pass would let the first viewer
// re-warm the cache for everyone behind it.
func evictFromCache(path string) error {
	f, err := os.Open(path) //nolint:gosec // an operator-configured library path the server already streams from
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
}
