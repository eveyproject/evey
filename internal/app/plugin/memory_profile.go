package plugin

import (
	"context"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
)

// KeyMemoryProfile is the server_config row selecting the memory profile.
// Values: "auto" (default - decide from the detected budget), "normal", "low".
// The low profile trades background comfort for RAM: idle plugin processes are
// suspended and resumed on demand (a suspended plugin's own background polling
// pauses with it until the next interaction).
const KeyMemoryProfile = "server.memory_profile"

const (
	MemoryProfileAuto   = "auto"
	MemoryProfileNormal = "normal"
	MemoryProfileLow    = "low"
)

// autoLowThresholdBytes is where "auto" flips to the low profile: a budget
// under 3 GiB (small NAS territory - 2 GiB boxes minus system overhead) can't
// afford always-on comfort processes.
const autoLowThresholdBytes int64 = 3 << 30

// memoryBudget is a test seam over sysinfo.MemoryBudgetBytes.
var memoryBudget = sysinfo.MemoryBudgetBytes

// ResolveMemoryProfile returns the effective profile ("normal"|"low") plus the
// detected memory budget in bytes (0 when undetectable). The stored setting
// wins; absent/invalid/"auto" resolves from the budget. A nil repo behaves as
// "auto".
func ResolveMemoryProfile(ctx context.Context, repo shared.ServerConfigRepository) (profile string, budgetBytes int64) {
	budget, ok := memoryBudget()
	if !ok {
		budget = 0
	}

	stored := ""
	if repo != nil {
		if entry, err := repo.Get(ctx, KeyMemoryProfile); err == nil && entry != nil {
			stored = entry.Value
		}
	}
	switch stored {
	case MemoryProfileNormal, MemoryProfileLow:
		return stored, budget
	case "", MemoryProfileAuto:
		// fall through to auto resolution
	default:
		slog.WarnContext(ctx, "invalid memory profile setting, resolving as auto",
			slog.String("key", KeyMemoryProfile), slog.String("value", stored))
	}
	if budget > 0 && budget < autoLowThresholdBytes {
		return MemoryProfileLow, budget
	}
	return MemoryProfileNormal, budget
}
