package plugin

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	domainplugin "github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
)

const lastErrorCacheSize = 50

// SecurityDashboardBus is the minimal event bus slice needed by SecurityDashboardService.
type SecurityDashboardBus interface {
	Subscribe(eventName string, handler func(ctx context.Context, event shared.Event)) string
}

// SandboxStatus is the host-level sandbox diagnosis shown on the security
// dashboard: configured vs effective mode plus the host probes explaining a
// degradation (an admin on Docker Desktop or an old NAS otherwise only sees a
// boot-time WARN log they will never read).
type SandboxStatus struct {
	Configured       string `json:"configured"`
	Effective        string `json:"effective"`
	LandlockKernel   bool   `json:"landlockKernel"`
	LauncherPresent  bool   `json:"launcherPresent"`
	BubblewrapUsable bool   `json:"bubblewrapUsable"`
}

// SecurityDashboardService aggregates security-relevant data for all installed plugins.
// lastError events are stored in an in-memory bounded map (tail-50 per plugin),
// since the domain has no persistent error-log repository.
type SecurityDashboardService struct {
	plugins         domainplugin.PluginRepository
	sandboxModeFn   func() string        // returns the effective sandbox mode in force (resolves "auto")
	sandboxStatusFn func() SandboxStatus // optional richer host diagnosis (see WithSandboxStatus)

	mu            sync.RWMutex
	lastErrors    map[string]*domainplugin.PluginLastError // pluginID → last error
	lastStartedAt map[string]time.Time                     // pluginID → last started timestamp
}

// NewSecurityDashboardService creates the service and subscribes to plugin events on bus.
// sandboxModeFn returns the effective sandbox mode in force (the runner resolves
// "auto" to landlock/bubblewrap/basic), read live so it reflects runtime changes.
func NewSecurityDashboardService(
	plugins domainplugin.PluginRepository,
	bus SecurityDashboardBus,
	sandboxModeFn func() string,
) *SecurityDashboardService {
	if sandboxModeFn == nil {
		sandboxModeFn = func() string { return "bubblewrap" }
	}
	s := &SecurityDashboardService{
		plugins:       plugins,
		sandboxModeFn: sandboxModeFn,
		lastErrors:    make(map[string]*domainplugin.PluginLastError),
		lastStartedAt: make(map[string]time.Time),
	}
	if bus != nil {
		bus.Subscribe("plugin.error", s.handlePluginError)
		bus.Subscribe("plugin.started", s.handlePluginStarted)
	}
	return s
}

// WithSandboxStatus wires the richer host sandbox diagnosis (configured mode,
// effective mode, probe results). Without it, SandboxStatus falls back to a
// minimal status built from sandboxModeFn.
func (s *SecurityDashboardService) WithSandboxStatus(fn func() SandboxStatus) *SecurityDashboardService {
	if fn != nil {
		s.sandboxStatusFn = fn
	}
	return s
}

// SandboxStatus returns the host-level sandbox diagnosis for the dashboard.
func (s *SecurityDashboardService) SandboxStatus() SandboxStatus {
	if s.sandboxStatusFn != nil {
		return s.sandboxStatusFn()
	}
	mode := s.sandboxModeFn()
	return SandboxStatus{Configured: mode, Effective: mode}
}

func (s *SecurityDashboardService) handlePluginError(_ context.Context, event shared.Event) {
	e, ok := event.(domainplugin.PluginError)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErrors[e.PluginID] = &domainplugin.PluginLastError{
		Ts:      e.OccurredAt,
		Message: e.ErrorMessage,
	}
}

func (s *SecurityDashboardService) handlePluginStarted(_ context.Context, event shared.Event) {
	e, ok := event.(domainplugin.PluginStarted)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastStartedAt[e.PluginID] = e.OccurredAt
}

// GetSecurityDashboard returns a security snapshot for every installed plugin.
func (s *SecurityDashboardService) GetSecurityDashboard(ctx context.Context) ([]domainplugin.PluginSecuritySnapshot, error) {
	plugins, err := s.plugins.List(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshots := make([]domainplugin.PluginSecuritySnapshot, 0, len(plugins))
	for _, p := range plugins {
		snap := domainplugin.PluginSecuritySnapshot{
			ID:          p.ID,
			Name:        p.Name,
			Version:     p.Version,
			SandboxMode: s.effectiveSandboxMode(p),
			Permissions: permissionsOrEmpty(p.Permissions),
			// INSTALL-TIME fact, not a live check: SourceHash is the SHA of the
			// installed ZIP, and the plugin dir is -rw with HOME/TMPDIR on it, so a
			// plugin can rewrite its own code without moving this value. SizeBytes
			// below is the only live field here.
			SourceSHA256: p.SourceHash,
			InstalledAt:  p.InstalledAt,
			SizeBytes:    dirSize(p.InstallDir),
		}

		// Findings from the static-analysis report stored at install time.
		if p.AnalysisReport != nil {
			snap.Findings = findingsFromReport(p.AnalysisReport)
		}

		// Signature status from cosign verification AT INSTALL TIME - never
		// re-verified at start. See the SourceSHA256 note above.
		snap.Signature = p.Signature

		// Last error from in-memory cache.
		if le, ok := s.lastErrors[p.ID]; ok {
			snap.LastError = le
		}

		// Last started timestamp from in-memory cache.
		if ts, ok := s.lastStartedAt[p.ID]; ok {
			t := ts
			snap.LastStartedAt = &t
		}

		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// effectiveSandboxMode returns the sandbox mode string for a given plugin.
// A plugin with the "subprocess" permission bypasses LD_PRELOAD sandboxing,
// so its effective mode is "none".
func (s *SecurityDashboardService) effectiveSandboxMode(p *domainplugin.Plugin) string {
	for _, perm := range p.Permissions {
		if perm == "subprocess" {
			return "none"
		}
	}
	return s.sandboxModeFn()
}

// findingsFromReport converts the domain AnalysisReport into SecurityFinding
// structs for the security dashboard.
//
// Preferred path: use RawFindings (populated at install time since the
// analyzer stores full per-finding data including description, file, line).
// Fallback path: reconstruct from Issues []string for legacy rows that were
// installed before RawFindings was added to the domain model.
func findingsFromReport(r *domainplugin.AnalysisReport) []domainplugin.SecurityFinding {
	if len(r.RawFindings) > 0 {
		findings := make([]domainplugin.SecurityFinding, 0, len(r.RawFindings))
		for _, f := range r.RawFindings {
			findings = append(findings, domainplugin.SecurityFinding{
				Category:    f.Category,
				Severity:    f.Severity,
				File:        f.File,
				Line:        f.Line,
				Description: f.Description,
			})
		}
		return findings
	}
	// Legacy fallback: reconstruct from formatted Issues strings.
	if len(r.Issues) == 0 {
		return nil
	}
	findings := make([]domainplugin.SecurityFinding, 0, len(r.Issues))
	for i, issue := range r.Issues {
		cat := ""
		if i < len(r.Categories) {
			cat = r.Categories[i]
		}
		findings = append(findings, domainplugin.SecurityFinding{
			Category:    cat,
			Severity:    parseSeverityFromIssue(issue),
			Description: issue, // full formatted string as description for legacy rows
		})
	}
	return findings
}

// parseSeverityFromIssue extracts "CRITICAL", "WARNING", or "INFO" from the
// formatted "[SEVERITY] file:line - description" issue string.
func parseSeverityFromIssue(issue string) string {
	if len(issue) < 2 {
		return "INFO"
	}
	if len(issue) > 10 && issue[0] == '[' {
		end := 1
		for end < len(issue) && issue[end] != ']' {
			end++
		}
		if end < len(issue) {
			return issue[1:end]
		}
	}
	return "INFO"
}

// permissionsOrEmpty returns an empty slice instead of nil for clean JSON output.
func permissionsOrEmpty(perms []string) []string {
	if perms == nil {
		return []string{}
	}
	return perms
}

// dirSize returns the total byte size of all files under dir using filepath.Walk.
// Returns 0 on error or if dir is empty.
func dirSize(dir string) int64 {
	if dir == "" {
		return 0
	}
	if _, err := os.Stat(dir); err != nil {
		return 0
	}
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !d.IsDir() {
			if info, infoErr := d.Info(); infoErr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("plugin dir size walk failed", slog.String("dir", dir), slog.Any("err", err))
	}
	return total
}
