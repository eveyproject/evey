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
// the SETUP itself is denied. For basic mode there is nothing to probe (nil).
func ProbeSandbox(mode string) error {
	if mode == sandboxModeBasic {
		return nil
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		return fmt.Errorf("bwrap binary not found in $PATH: %w", err)
	}
	tmp, err := os.MkdirTemp("", "evey-bwrap-probe-")
	if err != nil {
		return fmt.Errorf("create probe dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	// Reuse the real sandbox arg builder so the probe exercises the same
	// unshare + mount setup a plugin would. hasNetworkPerm=false → the
	// strictest form (also unshares the network namespace).
	prefix := buildBwrapCmd(tmp, "", false, mode, "")
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
