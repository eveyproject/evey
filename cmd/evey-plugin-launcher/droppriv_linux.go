//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

// dropPrivileges lowers the launcher - and therefore the plugin it execs - to
// the unprivileged plugin uid/gid. It is the FIRST thing run() does: before any
// OS thread is locked (Go's setuid is all-thread) and before the seccomp filter
// is installed (that filter denies setuid/setgid, so the plugin can never change
// identity afterwards).
//
// On the official non-root image the server runs as uid 1000 and the launcher
// carries the cap_setuid file capability, so it setuid's to 1001 itself. On a
// root server the parent already set the credential via SysProcAttr.Credential,
// so os.Getuid() already equals uid and this is a no-op.
//
// gid is set before uid (once non-root you can no longer change gid). The gid is
// the launcher's current group (the server's, e.g. evey) so the plugin stays in
// the shared group and can write the group-shared socket dir without any chown.
//
// Both uid and gid must be >= 1000. This refuses 0 (root) and service accounts,
// so a compromised server that controls the launcher argv cannot make it
// setuid to a privileged identity. Asking for the server's own uid is a no-op
// (no escalation), and uid <= 0 means "no drop requested".
//
// The plugin ends up with no capabilities regardless of the launcher's transient
// cap_setuid: the plugin binary has no file caps, we never raise inheritable or
// ambient caps, and Landlock/seccomp set PR_SET_NO_NEW_PRIVS before execve - so
// the cap recalculation at exec yields an empty set.
func dropPrivileges(uid, gid int) error {
	if uid <= 0 {
		return nil // no drop requested
	}
	if uid < 1000 || gid < 1000 {
		return fmt.Errorf("refusing unsafe target uid/gid %d/%d (must be >= 1000)", uid, gid)
	}
	if os.Getuid() == uid {
		return nil // already dropped (root parent set SysProcAttr.Credential)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid(%d): %w", gid, err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid(%d): %w (launcher needs the cap_setuid file capability; "+
			"incompatible with no-new-privileges)", uid, err)
	}
	return nil
}
