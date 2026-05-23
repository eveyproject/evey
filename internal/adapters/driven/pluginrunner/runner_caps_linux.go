//go:build linux

package pluginrunner

import (
	"os"
	"strconv"
	"strings"
)

// canSetUIDGID reports whether this process holds CAP_SETUID + CAP_SETGID in its
// effective set — i.e. it can drop a plugin child to a different uid/gid even
// while running non-root, when the binary is granted those caps via file
// capabilities (`setcap cap_setuid,cap_setgid+ep /evey`). Reading the real
// CapEff makes this self-correcting: if the caps aren't actually present, the
// caller leaves plugins at the server's uid rather than risking a setuid that
// would EPERM at spawn. The dropped-to-1001 child loses the caps automatically
// on the launcher execve (the launcher binary carries no file caps).
func canSetUIDGID() bool {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}
	return capEffHasSetUIDGID(string(data))
}

// capEffHasSetUIDGID parses /proc/<pid>/status content and reports whether the
// CapEff bitmask has both CAP_SETGID (6) and CAP_SETUID (7) set.
func capEffHasSetUIDGID(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		rest, ok := strings.CutPrefix(line, "CapEff:")
		if !ok {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(rest), 16, 64)
		if err != nil {
			return false
		}
		const capSetgid, capSetuid = 6, 7
		return mask&(1<<capSetgid) != 0 && mask&(1<<capSetuid) != 0
	}
	return false
}
