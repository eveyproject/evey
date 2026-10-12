package plugin

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

// DefaultPluginIdleTimeout is how long a plugin may stay interaction-free
// before the low-memory profile suspends its process. Interactions = host
// RPCs the plugin makes and widget asset fetches from browsers.
const DefaultPluginIdleTimeout = 15 * time.Minute

// idleSweepInterval is the cadence of the suspension sweep. Cheap (an
// in-memory map walk + one repo List), so a minute keeps latency bounded
// without mattering on the profile's target hardware.
const idleSweepInterval = time.Minute

// IdleManager suspends idle plugin processes when the low memory profile is
// active, and resumes them transparently on the next sign of interest (a
// widget asset fetch). Each idle node runtime holds 25-65 MB - on a 2 GB NAS
// that is the difference between a comfortable library scan and an OOM kill.
//
// Suspension is process-level only: the plugin keeps its Running status (it
// IS logically running, just parked), its permissions stay loaded, and the DB
// is never touched - so a server restart, AutoStartPlugins, and the admin UI
// all see the same world as before. The trade-off, documented on the setting:
// a suspended plugin's own background work (e.g. a scraper polling an
// external API) pauses until something looks at it again.
type IdleManager struct {
	plugins plugin.PluginRepository
	runner  plugin.PluginRunner
	timeout time.Duration
	now     func() time.Time // test seam

	mu        sync.Mutex
	enabled   bool
	last      map[string]time.Time // pluginID -> last interaction
	suspended map[string]bool      // pluginID -> parked by us
	// resuming marks a plugin whose resume I/O is in flight. It lets
	// EnsureStarted run its DB/RPC/spawn OUTSIDE m.mu (so Touch and every other
	// plugin's traffic no longer block behind a ~200ms cold resume) while still
	// preventing two concurrent resumes of the SAME plugin from double-starting
	// it - the single guarantee the old lock-across-I/O gave.
	resuming map[string]bool
}

func NewIdleManager(plugins plugin.PluginRepository, runner plugin.PluginRunner, timeout time.Duration) *IdleManager {
	if timeout <= 0 {
		timeout = DefaultPluginIdleTimeout
	}
	return &IdleManager{
		plugins:   plugins,
		runner:    runner,
		timeout:   timeout,
		now:       time.Now,
		last:      make(map[string]time.Time),
		suspended: make(map[string]bool),
		resuming:  make(map[string]bool),
	}
}

// SetEnabled flips idle management on (low profile) or off. Disabling resumes
// everything the manager suspended, so switching the profile back to normal
// restores the always-on world without a restart.
func (m *IdleManager) SetEnabled(ctx context.Context, on bool) {
	m.mu.Lock()
	if m.enabled == on {
		m.mu.Unlock()
		return
	}
	m.enabled = on
	var toResume []string
	if !on {
		for id := range m.suspended {
			toResume = append(toResume, id)
		}
	}
	m.mu.Unlock()

	slog.InfoContext(ctx, "plugin idle management", slog.Bool("enabled", on))
	for _, id := range toResume {
		m.EnsureStarted(ctx, id)
	}
}

// Touch records an interaction with the plugin (host RPC, asset fetch).
func (m *IdleManager) Touch(pluginID string) {
	m.mu.Lock()
	m.last[pluginID] = m.now()
	m.mu.Unlock()
}

// EnsureStarted resumes the plugin if this manager suspended it. Safe to call
// on any plugin in any state: it only acts on suspensions it owns, re-checks
// the DB (a plugin disabled while parked must stay down), and verifies via
// the runner that the process is not already back (e.g. an admin restart).
func (m *IdleManager) EnsureStarted(ctx context.Context, pluginID string) {
	// Claim the resume under the lock, then release it for the I/O. A plugin
	// stays in suspended[] until we succeed, so a concurrent Sweep keeps
	// skipping it (it already checks suspended[]); resuming[] is what stops a
	// second EnsureStarted from starting the same plugin twice.
	m.mu.Lock()
	if !m.suspended[pluginID] || m.resuming[pluginID] {
		m.mu.Unlock()
		return
	}
	m.resuming[pluginID] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.resuming, pluginID)
		m.mu.Unlock()
	}()

	// DB + RPC + process spawn, all off the lock: before this, every plugin
	// interaction on the server serialised behind one plugin's cold resume.
	p, err := m.plugins.GetByID(ctx, pluginID)
	if err != nil || p == nil || !p.Enabled || p.Status != plugin.Running || p.IsTheme() {
		m.mu.Lock()
		delete(m.suspended, pluginID) // no longer ours to manage
		m.mu.Unlock()
		return
	}
	if alive, herr := m.runner.Health(ctx, pluginID); herr == nil && alive {
		m.mu.Lock()
		delete(m.suspended, pluginID) // someone else already started it
		m.last[pluginID] = m.now()
		m.mu.Unlock()
		return
	}
	if err := m.runner.Start(ctx, p); err != nil {
		slog.WarnContext(ctx, "resume suspended plugin failed", slog.String("pluginID", pluginID), slog.Any("err", err))
		return // keep it marked suspended; the next interaction retries
	}
	slog.InfoContext(ctx, "resumed suspended plugin", slog.String("pluginID", pluginID))
	m.mu.Lock()
	delete(m.suspended, pluginID)
	m.last[pluginID] = m.now()
	m.mu.Unlock()
}

// Sweep suspends every eligible plugin whose last interaction is older than
// the idle timeout. A plugin never seen before gets its grace period started
// rather than being suspended immediately.
func (m *IdleManager) Sweep(ctx context.Context) {
	m.mu.Lock()
	if !m.enabled {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	all, err := m.plugins.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "idle sweep: list plugins", slog.Any("err", err))
		return
	}

	now := m.now()
	for _, p := range all {
		if !p.Enabled || p.Status != plugin.Running || p.IsTheme() {
			continue
		}
		m.mu.Lock()
		if !m.enabled || m.suspended[p.ID] {
			m.mu.Unlock()
			continue
		}
		lastSeen, known := m.last[p.ID]
		if !known {
			m.last[p.ID] = now // start the grace period
			m.mu.Unlock()
			continue
		}
		if now.Sub(lastSeen) < m.timeout {
			m.mu.Unlock()
			continue
		}
		m.suspended[p.ID] = true
		m.mu.Unlock()

		if err := m.runner.Stop(ctx, p.ID); err != nil {
			slog.WarnContext(ctx, "suspend idle plugin failed", slog.String("pluginID", p.ID), slog.Any("err", err))
			m.mu.Lock()
			delete(m.suspended, p.ID)
			m.mu.Unlock()
			continue
		}
		slog.InfoContext(ctx, "suspended idle plugin (low memory profile)",
			slog.String("pluginID", p.ID),
			slog.Duration("idle", now.Sub(lastSeen)))
	}
}

// Run drives periodic sweeps until ctx is canceled. Start it once at boot;
// when the profile is normal each tick is a cheap no-op.
func (m *IdleManager) Run(ctx context.Context) {
	ticker := time.NewTicker(idleSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Sweep(ctx)
		}
	}
}
