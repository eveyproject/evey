package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/bgloop"
)

// An installed plugin follows the repository it came from. Every hour the
// server reads the repository's latest release and, when it is newer, judges
// it before anything runs:
//
//   - refused: the analysis finds code it refuses (a critical finding, or a
//     warning the install itself blocks). Nothing is installed and the admins
//     are paged.
//   - held: the release asks for something the installed version does not
//     have - a permission, a capability, a slot, a host, a kind of analysis
//     warning it did not carry, another signing key or no signature - or the
//     installed version is not signed, or the admin turned automatic updates
//     off for this plugin. It waits for an admin, who is paged once per
//     release. An unsigned plugin never updates on its own: whoever takes
//     over its repository could ship code the analysis does not catch, and
//     only a signing key they do not hold stops them.
//   - applied: none of the above. It goes through the full install pipeline,
//     whose own checks still hold.

const (
	autoUpdateInterval   = time.Hour
	autoUpdateFirstCheck = 3 * time.Minute
	// One plugin's check may not run longer than this (download included).
	autoUpdateCheckTimeout = 10 * time.Minute
)

// ErrNoUpdateSource: the plugin follows no repository.
var ErrNoUpdateSource = errors.New("the plugin follows no repository")

// AutoUpdater runs the periodic check and the admin's "check now".
type AutoUpdater struct {
	installer  *InstallerService
	now        func() time.Time
	firstCheck time.Duration
	interval   time.Duration

	// checkMu serialises checks: the ticker and an admin's button.
	checkMu sync.Mutex
}

// NewAutoUpdater builds the updater on the installer it installs through.
func NewAutoUpdater(installer *InstallerService) *AutoUpdater {
	return &AutoUpdater{installer: installer, now: time.Now, firstCheck: autoUpdateFirstCheck, interval: autoUpdateInterval}
}

// WithSchedule replaces the delay before the first check and the interval
// between checks (tests).
func (u *AutoUpdater) WithSchedule(first, every time.Duration) *AutoUpdater {
	u.firstCheck, u.interval = first, every
	return u
}

// Start runs the check a few minutes after boot, then every hour, until ctx
// ends.
func (u *AutoUpdater) Start(ctx context.Context) {
	go func() {
		timer := time.NewTimer(u.firstCheck)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				bgloop.Tick("plugin.autoupdate", func() { u.CheckAll(ctx) })
				timer.Reset(u.interval)
			}
		}
	}()
}

// CheckAll checks every plugin that follows a repository.
func (u *AutoUpdater) CheckAll(ctx context.Context) {
	list, err := u.installer.plugins.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "plugin auto-update: list plugins", slog.Any("err", err))
		return
	}
	for _, p := range list {
		if ctx.Err() != nil {
			return
		}
		if p.UpdateSource == "" {
			continue
		}
		if _, err := u.Check(ctx, p.ID); err != nil {
			slog.WarnContext(ctx, "plugin auto-update: check", slog.String("plugin_id", p.ID), slog.Any("err", err))
		}
	}
}

// Check reads one plugin's latest release and acts on it. The returned state
// is also stored on the plugin.
func (u *AutoUpdater) Check(ctx context.Context, pluginID string) (plugin.UpdateState, error) {
	u.checkMu.Lock()
	defer u.checkMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, autoUpdateCheckTimeout)
	defer cancel()

	s := u.installer
	p, err := s.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return plugin.UpdateState{}, ErrPluginNotFound
	}
	if p.UpdateSource == "" {
		return plugin.UpdateState{}, ErrNoUpdateSource
	}
	if s.releases == nil {
		return plugin.UpdateState{}, ErrNoReleaseSource
	}
	state := plugin.UpdateState{CheckedAt: u.now().UTC(), Notified: p.UpdateState.Notified}

	rel, err := s.releases.Latest(ctx, p.UpdateSource)
	switch {
	case err != nil:
		state.Outcome, state.Reason = plugin.UpdateFailed, err.Error()
	case CompareSemver(rel.Version, p.Version) <= 0:
		state.LatestVersion, state.Outcome = rel.Version, plugin.UpdateUpToDate
	default:
		state.LatestVersion = rel.Version
		source := rel.ZipURL
		if rel.SHA256 != "" {
			source += "#sha256:" + rel.SHA256
		}
		state.Outcome, state.Reason = s.updateFromRelease(ctx, p, source, p.AutoUpdate)
	}

	if state.Outcome == plugin.UpdateHeld || state.Outcome == plugin.UpdateRefused {
		key := state.LatestVersion + "/" + string(state.Outcome)
		if state.Notified != key {
			state.Notified = key
			u.page(ctx, p, state)
		}
	}
	if err := s.plugins.SetUpdateState(ctx, p.ID, state); err != nil {
		return state, fmt.Errorf("record update state: %w", err)
	}
	slog.InfoContext(ctx, "plugin auto-update checked",
		slog.String("plugin_id", p.ID), slog.String("plugin", p.Name),
		slog.String("installed", p.Version), slog.String("latest", state.LatestVersion),
		slog.String("outcome", string(state.Outcome)), slog.String("reason", state.Reason))
	return state, nil
}

func (u *AutoUpdater) page(ctx context.Context, p *plugin.Plugin, state plugin.UpdateState) {
	ev := plugin.PluginUpdateAttention{
		PluginID: p.ID, Name: p.Name, Version: state.LatestVersion,
		Outcome: state.Outcome, Reason: state.Reason, OccurredAt: u.now(),
	}
	if err := u.installer.events.Publish(ctx, ev); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", ev.EventName()), slog.Any("err", err))
	}
}

// updateFromRelease downloads the release once, judges those bytes and, when
// apply is set and nothing in them is new and sensitive, installs those same
// bytes.
func (s *InstallerService) updateFromRelease(ctx context.Context, existing *plugin.Plugin, source string, apply bool) (plugin.UpdateOutcome, string) {
	localPath, expectedHash, cleanup, err := s.downloadIfURL(ctx, source)
	defer cleanup()
	if err != nil {
		return plugin.UpdateFailed, err.Error()
	}
	hash, err := hashFile(localPath)
	if err != nil {
		return plugin.UpdateFailed, err.Error()
	}
	if expectedHash != "" && hash != expectedHash {
		return plugin.UpdateFailed, fmt.Sprintf("the download does not match the release's sha256 (expected %s, got %s)", expectedHash, hash)
	}
	manifest, err := s.parseAndValidateManifest(localPath)
	if err != nil {
		return plugin.UpdateFailed, err.Error()
	}
	if manifest.Name != existing.Name {
		return plugin.UpdateRefused, fmt.Sprintf("the release is the plugin %q, not %q", manifest.Name, existing.Name)
	}
	if CompareSemver(manifest.Version, existing.Version) <= 0 {
		return plugin.UpdateFailed, fmt.Sprintf("the release's zip carries version %s, not newer than %s", manifest.Version, existing.Version)
	}

	report, err := s.analyzeCandidate(ctx, localPath, manifest)
	if err != nil {
		return plugin.UpdateHeld, "the release could not be analysed: " + err.Error()
	}
	if !report.Safe {
		return plugin.UpdateRefused, "the analysis refuses this code: " + criticalSummary(report)
	}
	if msg := blockingWarning(report, manifest.Permissions); msg != "" {
		return plugin.UpdateRefused, "the analysis refuses this code: " + msg
	}

	var held []string
	if err := checkPermissionEscalation(manifest, existing); err != nil {
		held = append(held, strings.TrimPrefix(err.Error(), ErrPluginPermissionEscalation.Error()+": "))
	}
	if added := newWarningCategories(existing.AnalysisReport, report); len(added) > 0 {
		held = append(held, "new kinds of analysis warnings: "+strings.Join(added, ", "))
	}
	if existing.PublisherKeyFingerprint == "" {
		held = append(held, "the installed version is not signed: an unsigned plugin updates only with an admin's approval")
	}
	if sig := s.candidateSignature(ctx, localPath, existing); sig != nil {
		switch sig.Continuity {
		case plugin.ContinuityChanged:
			held = append(held, "signed by another key than the installed version")
		case plugin.ContinuityLost:
			held = append(held, "no longer validly signed, the installed version was")
		}
	}
	if len(held) > 0 {
		return plugin.UpdateHeld, strings.Join(held, "; ")
	}
	if !apply {
		return plugin.UpdateHeld, "automatic updates are off for this plugin"
	}

	if _, err := s.installLocalZip(ctx, localPath, hash); err != nil {
		// A key the enforce policy no longer trusts is the admin's to settle;
		// it says nothing about the code, so it is held, never "refused".
		if errors.Is(err, plugin.ErrSignatureRefused) {
			return plugin.UpdateHeld, err.Error()
		}
		var blocked *PluginBlockedError
		if errors.As(err, &blocked) || strings.Contains(err.Error(), "plugin blocked") {
			return plugin.UpdateRefused, err.Error()
		}
		if errors.Is(err, ErrPluginPermissionEscalation) || errors.Is(err, ErrPublisherChanged) || errors.Is(err, ErrSignatureLost) {
			return plugin.UpdateHeld, err.Error()
		}
		return plugin.UpdateFailed, err.Error()
	}
	return plugin.UpdateApplied, ""
}

// analyzeCandidate runs the static analysis on an extracted copy of zipPath.
func (s *InstallerService) analyzeCandidate(ctx context.Context, zipPath string, manifest *plugin.PluginManifest) (*plugin.AnalysisReport, error) {
	if s.installer == nil || s.analyzer == nil || s.fs == nil {
		return nil, errors.New("no analyzer on this server")
	}
	tmpDir, err := s.fs.MkdirTemp("", "evey-plugin-autoupdate-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if rmErr := s.fs.RemoveAll(tmpDir); rmErr != nil {
			slog.WarnContext(ctx, "remove auto-update tmpdir", slog.Any("err", rmErr))
		}
	}()
	if err := s.installer.Extract(zipPath, tmpDir); err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	return s.analyzer.Analyze(tmpDir, manifest.WebRoot)
}

// newWarningCategories lists the warning categories the candidate carries and
// the installed version did not. A plugin installed before its report was
// kept has none on record, so every warning counts as new.
func newWarningCategories(installed, candidate *plugin.AnalysisReport) []string {
	had := map[string]bool{}
	if installed != nil {
		for _, f := range installed.RawFindings {
			had[f.Category] = true
		}
	}
	var added []string
	for _, f := range candidate.RawFindings {
		if f.Severity != "WARNING" && f.Severity != "CRITICAL" {
			continue
		}
		if !had[f.Category] && !slices.Contains(added, f.Category) {
			added = append(added, f.Category)
		}
	}
	return added
}

func criticalSummary(r *plugin.AnalysisReport) string {
	var cats []string
	for _, f := range r.RawFindings {
		if f.Severity == "CRITICAL" && !slices.Contains(cats, f.Category) {
			cats = append(cats, f.Category)
		}
	}
	if len(cats) == 0 {
		return "unsafe content"
	}
	return strings.Join(cats, ", ")
}
