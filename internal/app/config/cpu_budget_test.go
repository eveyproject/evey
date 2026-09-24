package config

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/pkg/cpubudget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCPUBudgetPolicy_DefaultsWhenNothingIsSet(t *testing.T) {
	t.Parallel()
	p := LoadCPUBudgetPolicy(context.Background(), fakes.NewFakeServerConfigRepository())
	assert.Equal(t, cpubudget.DefaultGlobalPercent, p.GlobalPercent)
	assert.Empty(t, p.TaskPercent)
	assert.Equal(t, cpubudget.DefaultGlobalPercent, LoadCPUBudgetPolicy(context.Background(), nil).GlobalPercent, "no store: default")
}

func TestLoadCPUBudgetPolicy_ReadsGlobalAndTaskShares(t *testing.T) {
	t.Parallel()
	repo := fakes.NewFakeServerConfigRepository()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetGlobal, "50"))
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetTaskPrefix+"trickplay_generation", "80"))
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetTaskPrefix+"library_scan", "25 %"))
	p := LoadCPUBudgetPolicy(ctx, repo)
	assert.Equal(t, 50, p.GlobalPercent)
	assert.Equal(t, map[string]int{"trickplay_generation": 80, "library_scan": 25}, p.TaskPercent)
}

func TestLoadCPUBudgetPolicy_RejectsOutOfRangeValues(t *testing.T) {
	t.Parallel()
	repo := fakes.NewFakeServerConfigRepository()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetGlobal, "80"), "the global share stops at the boost")
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetTaskPrefix+"intro_detection", "95"))
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetTaskPrefix+"complexity_classifier", "lots"))
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetTaskPrefix+"level_backfill", "0"))
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetTaskPrefix, "50"), "a task key without a name")
	p := LoadCPUBudgetPolicy(ctx, repo)
	assert.Equal(t, cpubudget.DefaultGlobalPercent, p.GlobalPercent)
	assert.Empty(t, p.TaskPercent)
}

// The two toggles the budget replaces keep their effect until the admin sets
// the new keys, which then win.
func TestLoadCPUBudgetPolicy_HonoursTheLegacyToggles(t *testing.T) {
	t.Parallel()
	repo := fakes.NewFakeServerConfigRepository()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, "trickplay.adaptive_cpu", "false"))
	require.NoError(t, repo.Set(ctx, "intro_detection.use_all_cores", "true"))
	p := LoadCPUBudgetPolicy(ctx, repo)
	assert.Equal(t, cpubudget.BoostGlobalPercent, p.GlobalPercent, "adaptive CPU off was the 50 % mode")
	assert.Equal(t, map[string]int{"intro_detection": cpubudget.MaxTaskPercent}, p.TaskPercent, "all cores was the intro boost")

	require.NoError(t, repo.Set(ctx, KeyCPUBudgetGlobal, "25"))
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetTaskPrefix+"intro_detection", "50"))
	p = LoadCPUBudgetPolicy(ctx, repo)
	assert.Equal(t, 25, p.GlobalPercent)
	assert.Equal(t, map[string]int{"intro_detection": 50}, p.TaskPercent)
}

func TestLoadCPUBudgetPolicy_UnreadableStoreKeepsTheDefault(t *testing.T) {
	t.Parallel()
	repo := fakes.NewFakeServerConfigRepository()
	repo.Err = errors.New("db down")
	p := LoadCPUBudgetPolicy(context.Background(), repo)
	assert.Equal(t, cpubudget.DefaultGlobalPercent, p.GlobalPercent)
	assert.Empty(t, p.TaskPercent)
}

func TestApplyCPUBudget_LoadsNowAndFollowsChangesLive(t *testing.T) {
	t.Parallel()
	repo := fakes.NewFakeServerConfigRepository()
	bus := fakes.NewFakeEventBus()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, KeyCPUBudgetGlobal, "50"))
	budget := cpubudget.New(8)
	ApplyCPUBudget(ctx, repo, bus, budget)
	assert.Equal(t, 4, budget.Slots("library_scan"), "loaded at once: 50 % of 8")

	svc := NewService(repo, WithEventBus(bus))
	require.NoError(t, svc.Set(ctx, KeyCPUBudgetTaskPrefix+"trickplay_generation", "80"))
	assert.Equal(t, 6, budget.Slots("trickplay_generation"), "a task key change lands live")
	assert.Equal(t, 6, budget.Ceiling())

	require.NoError(t, svc.Delete(ctx, KeyCPUBudgetGlobal))
	assert.Equal(t, 2, budget.Slots("library_scan"), "deleting the global key reverts to 25 %")

	require.NoError(t, svc.Set(ctx, "server.language", "fr"))
	assert.Equal(t, 6, budget.Slots("trickplay_generation"), "an unrelated key changes nothing")
	assert.True(t, IsCPUBudgetKey("trickplay.adaptive_cpu"), "the legacy toggles still drive a reload")
	assert.False(t, IsCPUBudgetKey("trickplay.enabled"))
}

func TestApplyCPUBudget_NilBudgetOrBusIsHarmless(t *testing.T) {
	t.Parallel()
	ApplyCPUBudget(context.Background(), fakes.NewFakeServerConfigRepository(), nil, nil)
	budget := cpubudget.New(4)
	ApplyCPUBudget(context.Background(), fakes.NewFakeServerConfigRepository(), nil, budget)
	assert.Equal(t, 1, budget.Slots("x"))
	var _ shared.EventBus = fakes.NewFakeEventBus()
}
