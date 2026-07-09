package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/plugin/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIdleFixture(t *testing.T) (*IdleManager, *fakes.FakePluginRepository, *fakes.FakePluginRunner, *plugin.Plugin) {
	t.Helper()
	repo := fakes.NewFakePluginRepository()
	runner := fakes.NewFakePluginRunner()
	p := &plugin.Plugin{ID: "plg-idle", Name: "idle", Enabled: true, Status: plugin.Running}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NoError(t, runner.Start(context.Background(), p))
	m := NewIdleManager(repo, runner, 10*time.Minute)
	m.SetEnabled(context.Background(), true)
	return m, repo, runner, p
}

func runnerAlive(t *testing.T, r *fakes.FakePluginRunner, id string) bool {
	t.Helper()
	alive, err := r.Health(context.Background(), id)
	require.NoError(t, err)
	return alive
}

func TestIdleManager_SuspendsAfterTimeoutAndResumesOnDemand(t *testing.T) {
	m, _, runner, p := newIdleFixture(t)
	ctx := context.Background()

	base := time.Now()
	m.now = func() time.Time { return base }
	m.Touch(p.ID)

	// Not yet idle: nothing happens.
	m.Sweep(ctx)
	assert.True(t, runnerAlive(t, runner, p.ID))

	// Past the timeout: suspended.
	m.now = func() time.Time { return base.Add(11 * time.Minute) }
	m.Sweep(ctx)
	assert.False(t, runnerAlive(t, runner, p.ID), "idle plugin should be suspended")

	// A widget view resumes it.
	m.EnsureStarted(ctx, p.ID)
	assert.True(t, runnerAlive(t, runner, p.ID), "suspended plugin should resume on demand")
}

func TestIdleManager_GracePeriodForUnseenPlugins(t *testing.T) {
	m, _, runner, p := newIdleFixture(t)
	ctx := context.Background()

	// Never touched: the first sweep only starts the grace period.
	m.Sweep(ctx)
	assert.True(t, runnerAlive(t, runner, p.ID), "first sweep must not suspend an unseen plugin")
}

func TestIdleManager_ActivityDefersSuspension(t *testing.T) {
	m, _, runner, p := newIdleFixture(t)
	ctx := context.Background()

	base := time.Now()
	m.now = func() time.Time { return base }
	m.Touch(p.ID)

	m.now = func() time.Time { return base.Add(9 * time.Minute) }
	m.Touch(p.ID) // host RPC arrives before the timeout

	m.now = func() time.Time { return base.Add(18 * time.Minute) }
	m.Sweep(ctx)
	assert.True(t, runnerAlive(t, runner, p.ID), "activity must reset the idle clock")
}

func TestIdleManager_DisabledProfileResumesSuspended(t *testing.T) {
	m, _, runner, p := newIdleFixture(t)
	ctx := context.Background()

	base := time.Now()
	m.now = func() time.Time { return base }
	m.Touch(p.ID)
	m.now = func() time.Time { return base.Add(11 * time.Minute) }
	m.Sweep(ctx)
	require.False(t, runnerAlive(t, runner, p.ID))

	// Switching back to the normal profile restores the always-on world.
	m.SetEnabled(ctx, false)
	assert.True(t, runnerAlive(t, runner, p.ID))

	// And further sweeps are no-ops.
	m.now = func() time.Time { return base.Add(60 * time.Minute) }
	m.Sweep(ctx)
	assert.True(t, runnerAlive(t, runner, p.ID))
}

func TestIdleManager_EnsureStartedIgnoresPluginsItDoesNotOwn(t *testing.T) {
	m, _, runner, p := newIdleFixture(t)
	ctx := context.Background()

	// Admin stopped it (status no longer Running, process down).
	require.NoError(t, runner.Stop(ctx, p.ID))
	p.Status = plugin.Stopped
	require.NoError(t, m.plugins.Update(ctx, p))

	m.EnsureStarted(ctx, p.ID)
	assert.False(t, runnerAlive(t, runner, p.ID), "EnsureStarted must not start plugins it did not suspend")
}

func TestIdleManager_DisabledWhileSuspendedStaysDown(t *testing.T) {
	m, repo, runner, p := newIdleFixture(t)
	ctx := context.Background()

	base := time.Now()
	m.now = func() time.Time { return base }
	m.Touch(p.ID)
	m.now = func() time.Time { return base.Add(11 * time.Minute) }
	m.Sweep(ctx)
	require.False(t, runnerAlive(t, runner, p.ID))

	// Admin disables the plugin while it is parked.
	p.Enabled = false
	p.Status = plugin.Stopped
	require.NoError(t, repo.Update(ctx, p))

	m.EnsureStarted(ctx, p.ID)
	assert.False(t, runnerAlive(t, runner, p.ID), "a disabled plugin must not be resurrected by a widget view")
}

func TestIdleManager_SweepSkipsThemesAndDisabled(t *testing.T) {
	m, repo, runner, _ := newIdleFixture(t)
	ctx := context.Background()

	theme := &plugin.Plugin{ID: "plg-theme", Name: "theme", Enabled: true, Status: plugin.Running, Kind: plugin.KindTheme}
	require.NoError(t, repo.Create(ctx, theme))

	base := time.Now()
	m.now = func() time.Time { return base }
	m.Sweep(ctx) // start grace periods
	m.now = func() time.Time { return base.Add(30 * time.Minute) }
	m.Sweep(ctx)

	// The theme was never a process; ensure the manager never tried to touch it.
	assert.False(t, m.suspended["plg-theme"])
	_ = runner
}
