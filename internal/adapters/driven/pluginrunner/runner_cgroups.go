package pluginrunner

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// validCgroupPluginID restricts pluginID to characters safe to embed in a
// filesystem path. This prevents path traversal (e.g. pluginID="../escape")
// from writing the cgroup outside the evey-plugins root.
var validCgroupPluginID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const (
	defaultMemBytes   int64 = 256 * 1024 * 1024 // 256 MiB
	defaultCPUQuota   int   = 50000              // 50 ms per period
	defaultCPUPeriod  int   = 100000             // 100 ms period → 50% of one core
	defaultPIDsMax    int   = 64

	// cgroupRoot is the mount point for cgroup v2. Overridable in tests via
	// setCgroupRoot so that test code never touches the real /sys/fs/cgroup.
	defaultCgroupRoot = "/sys/fs/cgroup"
)

// cgroupRoot is the effective cgroup v2 mount point. Tests override this
// with a t.TempDir() path so no privileged access is required.
var cgroupRoot = defaultCgroupRoot

// cgroupConfig holds resource limits for a single plugin cgroup.
// Zero values are replaced by the package defaults in prepareCgroup.
type cgroupConfig struct {
	memBytes  int64
	cpuQuota  int
	cpuPeriod int
	pidsMax   int
}

// defaults fills unset (zero) fields with sane package-level values.
func (c cgroupConfig) defaults() cgroupConfig {
	if c.memBytes == 0 {
		c.memBytes = defaultMemBytes
	}
	if c.cpuQuota == 0 {
		c.cpuQuota = defaultCPUQuota
	}
	if c.cpuPeriod == 0 {
		c.cpuPeriod = defaultCPUPeriod
	}
	if c.pidsMax == 0 {
		c.pidsMax = defaultPIDsMax
	}
	return c
}

// prepareCgroup creates a cgroup v2 hierarchy for the given plugin and writes
// the configured resource limits. It returns the cgroup directory path on
// success. If cgroup v2 is unavailable or the required controllers are
// missing, it returns ("", nil) so the caller continues without limits
// (fail-soft).
func prepareCgroup(pluginID string, cfg cgroupConfig) (string, error) {
	if !validCgroupPluginID.MatchString(pluginID) {
		return "", fmt.Errorf("pluginrunner/cgroup: invalid pluginID %q (allowed: %s)", pluginID, validCgroupPluginID.String())
	}
	cfg = cfg.defaults()

	controllersPath := filepath.Join(cgroupRoot, "cgroup.controllers")
	data, err := os.ReadFile(controllersPath)
	if err != nil {
		// cgroup v2 not available – not an error; caller will warn once.
		return "", nil
	}

	controllers := string(data)
	for _, required := range []string{"memory", "cpu", "pids"} {
		if !strings.Contains(controllers, required) {
			slog.Warn("cgroup v2 controller missing, resource limits disabled",
				slog.String("missing", required),
				slog.String("available", strings.TrimSpace(controllers)))
			return "", nil
		}
	}

	cgroupPath := filepath.Join(cgroupRoot, "evey-plugins", pluginID)
	if err := os.MkdirAll(cgroupPath, 0o755); err != nil {
		return "", fmt.Errorf("create plugin cgroup %s: %w", cgroupPath, err)
	}

	files := map[string]string{
		"memory.max": fmt.Sprintf("%d", cfg.memBytes),
		"cpu.max":    fmt.Sprintf("%d %d", cfg.cpuQuota, cfg.cpuPeriod),
		"pids.max":   fmt.Sprintf("%d", cfg.pidsMax),
	}
	for name, value := range files {
		p := filepath.Join(cgroupPath, name)
		if err := os.WriteFile(p, []byte(value), 0o644); err != nil {
			// Clean up the directory we just created before returning.
			_ = os.Remove(cgroupPath)
			return "", fmt.Errorf("write %s: %w", p, err)
		}
	}

	return cgroupPath, nil
}

// attachToCgroup writes pid into cgroupPath/cgroup.procs, moving the process
// into the cgroup. Must be called after cmd.Start so the PID is known.
func attachToCgroup(cgroupPath string, pid int) error {
	if cgroupPath == "" {
		return nil
	}
	procsPath := filepath.Join(cgroupPath, "cgroup.procs")
	return os.WriteFile(procsPath, []byte(fmt.Sprintf("%d", pid)), 0o644)
}

// cleanupCgroup removes the plugin cgroup directory. Linux refuses rmdir on a
// cgroup that still has live processes, so this is called after the process
// has exited. Errors are logged and swallowed — a leftover empty directory is
// not a security issue.
func cleanupCgroup(cgroupPath string) {
	if cgroupPath == "" {
		return
	}
	if err := os.Remove(cgroupPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("cleanup plugin cgroup", slog.String("path", cgroupPath), slog.Any("err", err))
	}
}
