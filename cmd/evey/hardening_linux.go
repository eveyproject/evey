//go:build linux

package main

import (
	"log/slog"

	"golang.org/x/sys/unix"
)

// hardenProcess marks the server process non-dumpable (PR_SET_DUMPABLE=0). This
// flips the owner of /proc/<pid>/{mem,environ,maps,…} to root, so another
// process sharing the server's UID cannot read them. It matters in the
// no-Landlock fallback (e.g. an old Synology kernel), where a plugin runs as
// the SAME uid as the server: without this, that plugin could read the server's
// in-memory JWT key and its environment secrets (DB_PASSWORD, MEILI_MASTER_KEY)
// via /proc. On Landlock kernels the plugin is already denied /proc entirely;
// this is the cross-kernel belt-and-suspenders. It does not affect the server
// reading its own files. Best-effort: a failure is logged, not fatal.
func hardenProcess() {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		slog.Warn("PR_SET_DUMPABLE=0 failed; server memory/env may be readable by a same-uid process",
			slog.Any("err", err))
	}
}
