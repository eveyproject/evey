package pluginrunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ProbeSandbox runs a throwaway bubblewrap sandbox to verify the host actually
// permits the namespace + mount operations bwrap needs. It exists because the
// usual failure mode — Docker's default seccomp profile blocking unshare/mount
// — only surfaces when the FIRST plugin tries to start and crashes with a
// cryptic "socket timeout". Probing once at boot turns that into an actionable
// warning.
//
// Returns nil when bwrap can build its sandbox (even if the trivial payload
// then fails to exec — that proves setup worked), or a descriptive error when
// the SETUP itself is denied. basic/auto never hard-fail; landlock fails when
// the kernel does not enforce Landlock.
func ProbeSandbox(mode string) error {
	switch mode {
	case sandboxModeBasic:
		return nil
	case sandboxModeLandlock:
		if !landlockAvailable() {
			return fmt.Errorf("Landlock is not enforced on this host (kernel <5.13, or Landlock absent from the active LSM list); " +
				"set EVEY_PLUGINS_SANDBOX_MODE=auto to fall back to the best available isolation")
		}
		return nil
	case sandboxModeAuto:
		// auto never hard-fails: resolveAutoMode always selects something. The
		// runner logs the resolved mode at the first plugin start.
		return nil
	}
	// bubblewrap / strict: verify the host actually permits the bwrap setup.
	if _, err := exec.LookPath("bwrap"); err != nil {
		return fmt.Errorf("bwrap binary not found in $PATH: %w", err)
	}
	tmp, err := os.MkdirTemp("", "evey-bwrap-probe-")
	if err != nil {
		return fmt.Errorf("create probe dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	// Reuse the real sandbox arg builder so the probe exercises the same
	// unshare + mount setup a plugin would (the network namespace is always
	// unshared now).
	prefix := buildBwrapCmd(tmp, "", mode)
	if prefix == nil { // mode resolved to basic
		return nil
	}
	args := append(prefix[1:], "true") // drop leading "bwrap"; payload exits 0 if it runs

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "bwrap", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	s := strings.TrimSpace(string(out))
	// "execvp ...: No such file" means bwrap built the sandbox fine and only
	// failed to resolve the trivial payload — that is a PASS for our purpose.
	if strings.Contains(s, "execvp") {
		return nil
	}
	if s == "" {
		s = err.Error()
	}
	return fmt.Errorf("%s", s)
}
