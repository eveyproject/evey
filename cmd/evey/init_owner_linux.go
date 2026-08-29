//go:build linux

package main

import (
	"os"
	"syscall"
)

// ownerOf returns the uid/gid owning path, or -1/-1 when the caller is not
// root (chown would fail anyway) or the owner cannot be read. `evey init` uses
// it to hand generated files to the operator who mounted the output directory.
func ownerOf(path string) (int, int) {
	if os.Geteuid() != 0 {
		return -1, -1
	}
	st, err := os.Stat(path)
	if err != nil {
		return -1, -1
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return -1, -1
	}
	return int(sys.Uid), int(sys.Gid)
}
