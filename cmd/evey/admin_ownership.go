package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// dirOwner returns the uid:gid owning dir.
func dirOwner(dir string) (uid, gid int, err error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return 0, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, syscall.ENOTSUP
	}
	return int(st.Uid), int(st.Gid), nil
}

// handBackToDirOwner re-owns the entries directly under dir (and dir itself
// when it differs) to the uid:gid that owns dir. `docker exec` bypasses the
// entrypoint's privilege drop, so an admin command runs as root and would leave
// root:root 0600 files the server (uid evey) can neither list nor restore.
// No-op unless root; a failure is logged, never fatal: the backup itself is
// already written.
func handBackToDirOwner(dir string) {
	if os.Geteuid() != 0 || dir == "" {
		return
	}
	uid, gid, err := dirOwner(dir)
	if err != nil || uid == 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("admin: cannot list directory to fix ownership", "dir", dir, "err", err)
		return
	}
	for _, e := range entries {
		chownToIfDifferent(filepath.Join(dir, e.Name()), uid, gid)
	}
}

// handBackToOwnerOf re-owns path to the uid:gid of ref (a file or dir). Same
// root-only, non-fatal contract as handBackToDirOwner.
func handBackToOwnerOf(path, ref string) {
	if os.Geteuid() != 0 {
		return
	}
	uid, gid, err := dirOwner(ref)
	if err != nil || uid == 0 {
		return
	}
	chownToIfDifferent(path, uid, gid)
}

func chownToIfDifferent(path string, uid, gid int) {
	fi, err := os.Lstat(path)
	if err != nil {
		return
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) == uid && int(st.Gid) == gid {
		return
	}
	if err := os.Lchown(path, uid, gid); err != nil {
		slog.Warn("admin: cannot hand file to the server user", "path", path, "err", err)
	}
}
