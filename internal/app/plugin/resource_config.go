package plugin

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// server_config rows holding the admin-editable per-plugin resource caps. They
// override the boot defaults (env vars read in internal/config) and are applied
// live: memory/PID/disk caps take effect on the next monitor sweep; cpu_nice and
// sandbox_mode apply to plugins started after the change. Editable from the
// admin panel (typed RPC) or directly via the generic server-config endpoint.
const (
	KeyMemoryMB    = "plugins.memory_mb"
	KeyPIDsMax     = "plugins.pids_max"
	KeyDiskMaxMB   = "plugins.disk_max_mb"
	KeyCPUNice     = "plugins.cpu_nice"
	KeySandboxMode = "plugins.sandbox_mode"
)

// ResourceConfigKeys is the set of server_config keys that drive the plugin
// runner; a ServerConfigChanged on any of them should re-apply the config.
var ResourceConfigKeys = map[string]bool{
	KeyMemoryMB:    true,
	KeyPIDsMax:     true,
	KeyDiskMaxMB:   true,
	KeyCPUNice:     true,
	KeySandboxMode: true,
}

// ResourceController is the subset of the plugin runner the applier drives.
// *pluginrunner.Runner satisfies it.
type ResourceController interface {
	SetResourceLimits(memBytes int64, pidsMax int)
	SetCPUNice(nice int)
	SetDiskQuota(maxBytes int64)
	SetSandboxMode(mode string)
}

// ResourceDefaults is the boot baseline (env config) used for any cap without a
// stored override.
type ResourceDefaults struct {
	MemoryMB    int
	PIDsMax     int
	DiskMaxMB   int
	CPUNice     int
	SandboxMode string
}

// ResourceValues is the effective config after overlaying stored overrides on
// the defaults. Returned to the admin UI and pushed into the runner.
type ResourceValues struct {
	MemoryMB    int
	PIDsMax     int
	DiskMaxMB   int
	CPUNice     int
	SandboxMode string
}

// validSandboxModes mirrors the runner's accepted modes; an unknown value falls
// back to the default rather than being pushed to the runner.
var validSandboxModes = map[string]bool{
	"auto": true, "landlock": true, "bubblewrap": true, "strict": true, "basic": true,
}

// IsValidSandboxMode reports whether mode is one the runner accepts. Used by the
// admin handler to reject bad input before persisting.
func IsValidSandboxMode(mode string) bool { return validSandboxModes[mode] }

// ResolveResourceConfig overlays stored overrides (server_config) on the boot
// defaults and returns the effective values. Invalid/out-of-range stored values
// are ignored (the default wins) and logged - a bad row must never break plugin
// startup. A nil repo yields the defaults unchanged.
func ResolveResourceConfig(ctx context.Context, repo shared.ServerConfigRepository, def ResourceDefaults) ResourceValues {
	v := ResourceValues(def)
	if repo == nil {
		return v
	}
	v.MemoryMB = overrideInt(ctx, repo, KeyMemoryMB, v.MemoryMB, 0, 1<<20)    // >=0, sane upper bound (1 TiB in MiB)
	v.PIDsMax = overrideInt(ctx, repo, KeyPIDsMax, v.PIDsMax, 0, 1<<20)       // >=0
	v.DiskMaxMB = overrideInt(ctx, repo, KeyDiskMaxMB, v.DiskMaxMB, 0, 1<<20) // >=0
	v.CPUNice = overrideInt(ctx, repo, KeyCPUNice, v.CPUNice, 0, 19)          // nice 0-19
	if m := overrideStr(ctx, repo, KeySandboxMode); m != "" {
		if validSandboxModes[m] {
			v.SandboxMode = m
		} else {
			slog.WarnContext(ctx, "plugin resource config: invalid sandbox mode, keeping default",
				slog.String("value", m), slog.String("default", v.SandboxMode))
		}
	}
	return v
}

// ApplyResourceConfig resolves the effective config and pushes it into the
// runner. Called at boot (so saved overrides take effect immediately) and after
// any ServerConfigChanged on a plugins.* key (live reload, no restart).
func ApplyResourceConfig(ctx context.Context, repo shared.ServerConfigRepository, ctrl ResourceController, def ResourceDefaults) ResourceValues {
	v := ResolveResourceConfig(ctx, repo, def)
	if ctrl != nil {
		ctrl.SetResourceLimits(int64(v.MemoryMB)*1024*1024, v.PIDsMax)
		ctrl.SetDiskQuota(int64(v.DiskMaxMB) * 1024 * 1024)
		ctrl.SetCPUNice(v.CPUNice)
		ctrl.SetSandboxMode(v.SandboxMode)
	}
	slog.InfoContext(ctx, "plugin resource config applied",
		slog.Int("memory_mb", v.MemoryMB), slog.Int("pids_max", v.PIDsMax),
		slog.Int("disk_max_mb", v.DiskMaxMB), slog.Int("cpu_nice", v.CPUNice),
		slog.String("sandbox_mode", v.SandboxMode))
	return v
}

// overrideInt reads an integer server_config value, returning def when the key
// is absent/empty/unparsable/out-of-range [min,max].
func overrideInt(ctx context.Context, repo shared.ServerConfigRepository, key string, def, minV, maxV int) int {
	s := overrideStr(ctx, repo, key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < minV || n > maxV {
		slog.WarnContext(ctx, "plugin resource config: invalid value, keeping default",
			slog.String("key", key), slog.String("value", s), slog.Int("default", def))
		return def
	}
	return n
}

func overrideStr(ctx context.Context, repo shared.ServerConfigRepository, key string) string {
	entry, err := repo.Get(ctx, key)
	if err != nil || entry == nil {
		return ""
	}
	return entry.Value
}
