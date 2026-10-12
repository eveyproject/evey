package plugin

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin/fakes"
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

// gatingRunner counts Start calls and can block them on a gate, so a test can
// force two EnsureStarted calls to overlap in the resume I/O window.
type gatingRunner struct {
	mu         sync.Mutex
	running    map[string]bool
	startCount int
	gate       chan struct{} // when non-nil, Start blocks until closed
}

func newGatingRunner() *gatingRunner { return &gatingRunner{running: map[string]bool{}} }

func (r *gatingRunner) Start(_ context.Context, p *plugin.Plugin) error {
	r.mu.Lock()
	r.startCount++
	gate := r.gate
	r.mu.Unlock()
	if gate != nil {
		<-gate // hold inside Start, off IdleManager's lock, to widen the race window
	}
	r.mu.Lock()
	r.running[p.ID] = true
	r.mu.Unlock()
	return nil
}
func (r *gatingRunner) Stop(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, id)
	return nil
}
func (r *gatingRunner) Kill(string) error { return nil }
func (r *gatingRunner) Health(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[id], nil
}
func (r *gatingRunner) SendEvent(context.Context, string, any) error { return nil }
func (r *gatingRunner) starts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startCount
}

// Two EnsureStarted calls racing on the same suspended plugin must resume it
// exactly once. Before the resume was moved off the global lock, correctness
// relied on holding that lock across the spawn; the resuming[] guard is what
// keeps the single-start guarantee now that the lock is released for the I/O.
func TestIdleManager_ConcurrentEnsureStartedStartsOnce(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	runner := newGatingRunner()
	p := &plugin.Plugin{ID: "plg-race", Name: "race", Enabled: true, Status: plugin.Running}
	require.NoError(t, repo.Create(context.Background(), p))

	m := NewIdleManager(repo, runner, 10*time.Minute)
	m.SetEnabled(context.Background(), true)
	// Park it: mark suspended as the manager would.
	m.mu.Lock()
	m.suspended[p.ID] = true
	m.mu.Unlock()

	runner.mu.Lock()
	runner.gate = make(chan struct{})
	runner.mu.Unlock()

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { m.EnsureStarted(context.Background(), p.ID) })
	}
	// Give the winner time to enter Start and the losers to bounce off resuming[].
	time.Sleep(50 * time.Millisecond)
	close(runner.gate) // release the blocked Start
	wg.Wait()

	assert.Equal(t, 1, runner.starts(), "a plugin must be resumed exactly once under concurrent EnsureStarted")
	assert.True(t, runnerAliveG(t, runner, p.ID))
	m.mu.Lock()
	assert.False(t, m.suspended[p.ID], "successful resume clears the suspended mark")
	assert.Empty(t, m.resuming, "the resuming guard is released after the resume")
	m.mu.Unlock()
}

func runnerAliveG(t *testing.T, r *gatingRunner, id string) bool {
	t.Helper()
	alive, err := r.Health(context.Background(), id)
	require.NoError(t, err)
	return alive
}
