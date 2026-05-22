//go:build linux

package pluginrunner

import "golang.org/x/sys/unix"

// landlockAvailable reports whether the running kernel enforces Landlock, i.e.
// it is compiled in AND present in the active LSM list. Querying the supported
// ABI version (NULL attr + the VERSION flag) returns ≥1 when usable, or an
// errno (ENOSYS when the syscall is absent, EOPNOTSUPP when Landlock is not in
// the active LSM list) otherwise.
func landlockAvailable() bool {
	r, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION))
	return errno == 0 && int(r) >= 1
}
