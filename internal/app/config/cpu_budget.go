package config

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/cpubudget"
)

// The background CPU budget the admin sets under Server settings. The global
// share applies to every background task; a task key names one heavy task
// (its bgtasks name) and gives it a share of its own, which replaces the
// global one for that task. The budget itself (cpubudget) keeps the shares
// from stacking.
const (
	// KeyCPUBudgetGlobal holds the global share, "25" or "50".
	KeyCPUBudgetGlobal = "background.cpu_percent"
	// KeyCPUBudgetTaskPrefix + task name holds a task's own share, 1..80.
	KeyCPUBudgetTaskPrefix = "background.cpu_percent."

	// The two toggles this budget replaces. Still honoured when the new keys
	// are unset, so an admin who had flipped one keeps what they chose.
	legacyKeyTrickplayAdaptiveCPU = "trickplay.adaptive_cpu"
	legacyKeyIntroUseAllCores     = "intro_detection.use_all_cores"
	legacyIntroTask               = "intro_detection"
)

// LoadCPUBudgetPolicy reads the policy from the store. Unreadable or
// out-of-range values fall back to the default and are logged once here, so
// a typo in the raw entries table never turns into a silent full-speed scan.
func LoadCPUBudgetPolicy(ctx context.Context, repo shared.ServerConfigRepository) cpubudget.Policy {
	policy := cpubudget.Policy{GlobalPercent: cpubudget.DefaultGlobalPercent}
	if repo == nil {
		return policy
	}
	entries, err := repo.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "cpu budget: config unreadable, keeping the default", slog.Any("err", err))
		return policy
	}
	values := make(map[string]string, len(entries))
	for _, e := range entries {
		if e != nil {
			values[e.Key] = e.Value
		}
	}
	if v, ok := values[KeyCPUBudgetGlobal]; ok && v != "" {
		if pct, ok := parsePercent(v, cpubudget.BoostGlobalPercent); ok {
			policy.GlobalPercent = pct
		} else {
			slog.WarnContext(ctx, "cpu budget: ignoring global share", slog.String("value", v))
		}
	} else if values[legacyKeyTrickplayAdaptiveCPU] == "false" {
		policy.GlobalPercent = cpubudget.BoostGlobalPercent
	}
	for key, v := range values {
		task, ok := strings.CutPrefix(key, KeyCPUBudgetTaskPrefix)
		if !ok || task == "" || v == "" {
			continue
		}
		pct, ok := parsePercent(v, cpubudget.MaxTaskPercent)
		if !ok {
			slog.WarnContext(ctx, "cpu budget: ignoring task share", slog.String("task", task), slog.String("value", v))
			continue
		}
		if policy.TaskPercent == nil {
			policy.TaskPercent = make(map[string]int)
		}
		policy.TaskPercent[task] = pct
	}
	if _, set := policy.TaskPercent[legacyIntroTask]; !set && values[legacyKeyIntroUseAllCores] == "true" {
		if policy.TaskPercent == nil {
			policy.TaskPercent = make(map[string]int)
		}
		policy.TaskPercent[legacyIntroTask] = cpubudget.MaxTaskPercent
	}
	return policy.Normalized()
}

func parsePercent(v string, maxPct int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "%")))
	if err != nil || n < 1 || n > maxPct {
		return 0, false
	}
	return n, true
}

// IsCPUBudgetKey reports whether a config key feeds the CPU budget policy.
func IsCPUBudgetKey(key string) bool {
	return key == KeyCPUBudgetGlobal || strings.HasPrefix(key, KeyCPUBudgetTaskPrefix) ||
		key == legacyKeyTrickplayAdaptiveCPU || key == legacyKeyIntroUseAllCores
}

// ApplyCPUBudget loads the policy into budget now and keeps it current: every
// change to a budget key re-reads the store and swaps the policy live, so the
// admin's choice takes effect on the next item without a restart.
func ApplyCPUBudget(ctx context.Context, repo shared.ServerConfigRepository, bus shared.EventBus, budget *cpubudget.Budget) {
	if budget == nil {
		return
	}
	budget.SetPolicy(LoadCPUBudgetPolicy(ctx, repo))
	if bus == nil {
		return
	}
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(evtCtx context.Context, ev shared.Event) {
		changed, ok := ev.(shared.ServerConfigChanged)
		if !ok || !IsCPUBudgetKey(changed.Key) {
			return
		}
		policy := LoadCPUBudgetPolicy(evtCtx, repo)
		budget.SetPolicy(policy)
		slog.InfoContext(evtCtx, "cpu budget: policy updated",
			slog.String("key", changed.Key),
			slog.Int("global_percent", policy.GlobalPercent),
			slog.Any("task_percent", policy.TaskPercent),
			slog.Int("ceiling_cores", budget.Ceiling()))
	})
}
