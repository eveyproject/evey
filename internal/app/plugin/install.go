package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/themejs"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// install.go holds the plugin install/upgrade write pipeline: source download,
// manifest validation, integrity + signature checks, staged extraction, static
// security analysis, the atomic swap-into-place and runner start. Every stage
// is a method so InstallPlugin reads as an orchestration and each check is
// independently testable.

// uninstallRetryDelays are the back-off intervals used between RemoveAll
// attempts during the in-place upgrade swap. Three attempts total
// (initial + two retries).
var uninstallRetryDelays = [2]time.Duration{500 * time.Millisecond, 2 * time.Second}

// nativeBinaryPerm is the manifest permission a plugin must declare to
// be allowed to ship compiled binaries or embedded archives. Without it
// the installer rejects any plugin that the static analyzer flagged
// with `compiled_binary` or `embedded_archive` categories - opaque blobs
// the regex scanner cannot inspect.
const nativeBinaryPerm = "native-binary"

// gpuPermission is the manifest permission that grants a plugin the GPU render
// node. It is the highest-risk permission Evey exposes: a render fd is a direct
// kernel-driver ioctl surface and a per-GPU (not per-tenant) memory boundary
// shared with Evey's own transcoder. The installer surfaces a loud, specific
// CRITICAL warning at preview so the admin's grant is informed and deliberate.
const gpuPermission = "hardware.gpu"

// gpuPermissionWarning is shown at install/update preview whenever a plugin
// declares hardware.gpu, so the admin consents to the residual risk explicitly.
const gpuPermissionWarning = "[CRITICAL] permission hardware.gpu - grants the GPU render node: a kernel-driver attack surface and a GPU-memory boundary SHARED with the transcoder (cross-process memory leak possible on AMD/Apple/Qualcomm GPUs; a hung GPU can stall transcoding). Grant ONLY to first-party / trusted plugins; never to an untrusted source. See docs/plugins.md."

// PluginBlockedError is returned by InstallPlugin when the static analyzer
// rejects a plugin for CRITICAL security findings. It carries the structured
// data the http layer needs: the pre-formatted Issues strings (kept for
// backwards-compat with the admin UI which already renders them) AND the
// deduplicated Categories list (machine-readable enums like "shell_exec",
// "sandbox_escape", etc.) which the security-spec test asserts against.
//
// Other "plugin blocked: ..." returns (analyzer-failed, theme.js scanner,
// native-binary opt-in, escalated warnings) do NOT use this type because they
// don't have a per-finding category list - the http handler falls back to its
// existing string-parsing path for those.
type PluginBlockedError struct {
	Issues     []string
	Categories []string
}

// Refusal sentinels: the archive or its address is the problem, not the
// server. The API maps them to the caller's fault with the reason attached;
// anything else on the install path stays a sanitized internal error.
var (
	ErrInvalidManifest   = errors.New("invalid plugin manifest")
	ErrHashMismatch      = errors.New("plugin hash mismatch")
	ErrSourceUnavailable = errors.New("plugin source unavailable")
)

func (e *PluginBlockedError) Error() string {
	return fmt.Sprintf("plugin blocked: critical security issues found: %v", e.Issues)
}

// downloadIfURL detects if source is an HTTP(S) URL, downloads it into a
// temp file with a 100 MB limit and returns the local path + cleanup func.
// The fragment "#sha256:<hex>" is parsed as the expected hash if present and
// stripped from the URL before the download.
// For local paths, returns source unchanged + noop cleanup + "" hash.
func (s *InstallerService) downloadIfURL(ctx context.Context, source string) (localPath, expectedHash string, cleanup func(), err error) {
	cleanup = func() {}

	// Parse optional "#sha256:<hex>" fragment.
	rawURL, fragment, _ := strings.Cut(source, "#")
	if h, ok := strings.CutPrefix(fragment, "sha256:"); ok {
		expectedHash = h
		source = rawURL
	}

	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		localPath = source
		return
	}

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if reqErr != nil {
		err = fmt.Errorf("build download request: %w", reqErr)
		return
	}
	req.Header.Set("User-Agent", "evey-plugin-installer/1.0")

	resp, doErr := s.httpClient.Do(req)
	if doErr != nil {
		err = fmt.Errorf("download plugin: %w", doErr)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("download plugin: HTTP %d", resp.StatusCode)
		return
	}

	tmp, tmpErr := os.CreateTemp("", "evey-plugin-dl-*.zip")
	if tmpErr != nil {
		err = fmt.Errorf("create temp file: %w", tmpErr)
		return
	}
	localPath = tmp.Name()
	cleanup = func() {
		tmp.Close()
		os.Remove(localPath) //nolint:errcheck
	}

	const maxBytes = 100 << 20 // 100 MB
	if _, cpErr := io.Copy(tmp, io.LimitReader(resp.Body, maxBytes+1)); cpErr != nil {
		cleanup()
		cleanup = func() {}
		localPath = ""
		err = fmt.Errorf("write plugin download: %w", cpErr)
		return
	}
	if fi, statErr := tmp.Stat(); statErr == nil && fi.Size() > maxBytes {
		cleanup()
		cleanup = func() {}
		localPath = ""
		err = fmt.Errorf("plugin zip exceeds 100 MB limit")
		return
	}
	tmp.Close()
	return
}

func (s *InstallerService) InstallPlugin(ctx context.Context, source string) (*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.InstallPlugin")
	defer span.End()

	localPath, expectedHash, cleanupDL, dlErr := s.downloadIfURL(ctx, source)
	defer cleanupDL()
	if dlErr != nil {
		return nil, fmt.Errorf("resolve plugin source: %w: %w", ErrSourceUnavailable, dlErr)
	}
	return s.installLocalZip(ctx, localPath, expectedHash)
}

// installLocalZip runs the whole install pipeline (manifest, integrity and
// signature, extraction, static analysis, swap) on an already-local zip, so a
// caller that hashed a download installs those exact bytes, never a re-fetch.
func (s *InstallerService) installLocalZip(ctx context.Context, zipPath, expectedHash string) (*plugin.Plugin, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()

	manifest, err := s.parseAndValidateManifest(zipPath)
	if err != nil {
		return nil, err
	}

	existing, err := s.resolveExistingPlugin(ctx, manifest)
	if err != nil {
		return nil, err
	}

	sourceHash, sigStatus, err := s.verifyPluginIntegrity(ctx, zipPath, expectedHash)
	if err != nil {
		return nil, err
	}

	p, err := s.buildPluginRecord(existing, sourceHash, manifest, sigStatus)
	if err != nil {
		return nil, err
	}

	finalDir, stagingDir, err := s.extractPluginToStaging(p, existing, zipPath)
	if err != nil {
		return nil, err
	}

	// installOK gates the deferred cleanup: any error return after this point
	// removes the STAGING dir, ensuring no orphaned dirs accumulate from failed
	// installs. Once the validated staging dir has been swapped into finalDir
	// (swapped=true) the deferred cleanup must not touch it - that would delete
	// the just-installed plugin.
	installOK := false
	swapped := false
	defer func() {
		if !installOK && !swapped && p.InstallDir != "" && s.fs != nil {
			s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
		}
	}()

	if err = s.validateStagedPlugin(ctx, p); err != nil {
		return nil, err
	}

	swapped, err = s.swapAndPersistPlugin(ctx, p, existing, stagingDir, finalDir)
	if err != nil {
		return nil, err
	}

	if startFailed := s.startPluginAndUpdateStatus(ctx, p); startFailed {
		installOK = true
		return p, nil
	}

	evtInstalled := plugin.PluginInstalled{PluginID: p.ID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtInstalled); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtInstalled.EventName()), slog.Any("err", err))
	}
	installOK = true
	return p, nil
}

// parseAndValidateManifest parses the plugin manifest from zipPath, validates
// it, and fails fast if the host image lacks the declared runtime. Themes
// have no backend language and bypass the runtime check. The checker is
// optional - callers that don't wire it (legacy tests) keep the previous
// behaviour.
func (s *InstallerService) parseAndValidateManifest(zipPath string) (*plugin.PluginManifest, error) {
	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w: %w", ErrInvalidManifest, err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w: %w", ErrInvalidManifest, err)
	}

	if s.runtimes != nil && manifest.Kind != plugin.KindTheme && manifest.Language != "" {
		if !s.runtimes.IsAvailable(manifest.Language) {
			return nil, fmt.Errorf(
				"plugin requires %q runtime which is not installed in this image; available runtimes: %v - install the matching image variant (e.g. evey:beta-nodejs for node, evey:beta-full for all runtimes)",
				manifest.Language, s.runtimes.Available(),
			)
		}
	}
	return manifest, nil
}

// resolveExistingPlugin looks up an already-installed plugin with the same
// name and, if found, enforces the downgrade and permission-escalation
// guards. Returns nil, nil when no plugin with this name is installed yet.
func (s *InstallerService) resolveExistingPlugin(ctx context.Context, manifest *plugin.PluginManifest) (*plugin.Plugin, error) {
	existing, getErr := s.plugins.GetByName(ctx, manifest.Name)
	if getErr != nil && !errors.Is(getErr, shared.ErrNotFound) {
		// A real lookup failure must not be treated as "no existing plugin":
		// that would skip the downgrade and permission-escalation guards and
		// let a fresh install silently clobber an already-installed one.
		return nil, fmt.Errorf("look up existing plugin %q: %w", manifest.Name, getErr)
	}
	if existing != nil {
		cmp := CompareSemver(manifest.Version, existing.Version)
		if cmp <= 0 {
			return nil, fmt.Errorf("plugin already installed at version %s, new version must be higher", existing.Version)
		}
		if !allowPermissionEscalation(ctx) {
			if err := checkPermissionEscalation(manifest, existing); err != nil {
				return nil, err
			}
		}
		// In-place upgrade: reuse the existing id and Update the row (below)
		// instead of delete+recreate. plugin_data (per-user/profile config)
		// FK-cascades on plugins(id) ON DELETE, so deleting the row would
		// silently wipe every user's/profile's settings on every update. The new
		// version is staged in a sibling dir and validated there; only once it
		// passes every check do we tear down the old runtime and swap it in (see
		// the swap below), so a FAILED upgrade rolls back to the old version with
		// its files, row and config fully intact.
		slog.InfoContext(ctx, "upgrading plugin in place", slog.String("name", manifest.Name),
			slog.String("id", existing.ID), slog.String("from", existing.Version), slog.String("to", manifest.Version))
	}
	return existing, nil
}

// verifyPluginIntegrity hashes the plugin zip (checking it against
// expectedHash when the source URL carried a "#sha256:" fragment) and runs
// cosign signature verification. Policy controls whether a verification
// failure blocks the install.
func (s *InstallerService) verifyPluginIntegrity(ctx context.Context, zipPath, expectedHash string) (sourceHash string, sigStatus *plugin.SignatureStatus, err error) {
	sourceHash, hashErr := hashFile(zipPath)
	if hashErr != nil {
		return "", nil, fmt.Errorf("hash plugin file: %w", hashErr)
	}
	if expectedHash != "" && sourceHash != expectedHash {
		return "", nil, fmt.Errorf("%w: expected %s, got %s", ErrHashMismatch, expectedHash, sourceHash)
	}

	if s.sigVerifier != nil {
		sigStatus, err = s.sigVerifier.VerifySignature(ctx, zipPath, "")
		if err != nil {
			// Only reached when policy is "enforce" and verification failed.
			return "", nil, fmt.Errorf("plugin blocked: %w", err)
		}
	}
	return sourceHash, sigStatus, nil
}

// buildPluginRecord constructs the domain plugin.Plugin from the parsed
// manifest, reusing the existing id (and carrying over server-level config +
// enabled state) on an upgrade, or minting a fresh id for a new install.
func (s *InstallerService) buildPluginRecord(existing *plugin.Plugin, sourceHash string, manifest *plugin.PluginManifest, sigStatus *plugin.SignatureStatus) (*plugin.Plugin, error) {
	// Reuse the id on upgrade so the row can be Updated in place (keeping
	// plugin_data); mint a fresh one only for a genuinely new install.
	pluginID := id.New("plg-")
	if existing != nil {
		pluginID = existing.ID
	}
	p, err := plugin.NewPluginFromManifest(pluginID, sourceHash, manifest)
	if err != nil {
		return nil, fmt.Errorf("build plugin: %w", err)
	}

	// Defense-in-depth: a kind:plugin without a backend language is nonsensical -
	// use kind:theme for front-only customization.
	if p.Kind == plugin.KindPlugin && p.Language == "" {
		return nil, errors.New("kind:plugin requires a backend; for front-only customization use kind:theme")
	}

	p.Signature = sigStatus

	if existing != nil {
		// Carry the server-level config the admin set and the enabled state
		// across the update - the manifest resets Config to empty and Enabled to
		// true. (Per-user/profile config lives in plugin_data and is preserved by
		// the reused id.) The new ConfigSchema still comes from the manifest, so
		// added fields appear; unchanged fields keep their stored values.
		p.Config = existing.Config
		p.Enabled = existing.Enabled
	}
	return p, nil
}

// extractPluginToStaging computes the plugin's canonical dir and, on
// upgrade, a sibling staging dir, then extracts the zip into it. On upgrade
// the extraction lands in the staging dir so a failed upgrade never touches
// the live version; on a fresh install the staging dir IS the final dir
// (nothing to protect). Sets p.InstallDir on success.
func (s *InstallerService) extractPluginToStaging(p *plugin.Plugin, existing *plugin.Plugin, zipPath string) (finalDir, stagingDir string, err error) {
	finalDir = filepath.Join(s.pluginDir, p.ID)
	stagingDir = finalDir
	if existing != nil {
		stagingDir = finalDir + ".upgrade"
	}
	if s.installer != nil {
		// Clear any leftover staging dir from a previously-interrupted upgrade.
		if stagingDir != finalDir && s.fs != nil {
			s.fs.RemoveAll(stagingDir) //nolint:errcheck
		}
		if err := s.installer.Extract(zipPath, stagingDir); err != nil {
			return finalDir, stagingDir, fmt.Errorf("extract plugin: %w", err)
		}
		p.InstallDir = stagingDir
	}
	return finalDir, stagingDir, nil
}

// validateStagedPlugin runs every check that operates on the extracted
// staging dir: theme.css sanitization, theme.js forbidden-pattern scanning,
// SDK stub copy, ownership fixup, and static security analysis.
func (s *InstallerService) validateStagedPlugin(ctx context.Context, p *plugin.Plugin) error {
	if p.InstallDir != "" && p.IsTheme() && s.cssSanitizer != nil {
		themePath := filepath.Join(p.InstallDir, "theme.css")
		raw, readErr := os.ReadFile(themePath)
		// A present-but-unreadable theme.css must NOT skip sanitization (that
		// would serve it raw); only a genuinely-absent file is fine to skip.
		if readErr != nil && !os.IsNotExist(readErr) {
			return fmt.Errorf("read theme.css for sanitization: %w", readErr)
		}
		if readErr == nil {
			sanitized, warns := s.cssSanitizer.SanitizeCSS(raw)
			if len(warns) > 0 {
				slog.WarnContext(ctx, "css sanitizer cleaned theme at install", slog.String("pluginID", p.ID), slog.Any("warnings", warns))
				// The write-back is the only sanitization barrier - theme_asset
				// serves theme.css raw (and pre-auth on the login page). If it
				// fails, fail the install so the deferred cleanup removes the dir
				// rather than leaving the dangerous original on disk.
				if writeErr := os.WriteFile(themePath, sanitized, 0o644); writeErr != nil { //nolint:gosec // G703 FP: themePath = pluginDir/<plugin-id>/theme.css, both components server-controlled, filename is a literal - no user-tainted path element
					return fmt.Errorf("persist sanitized theme.css: %w", writeErr)
				}
			}
		}
	}

	if p.InstallDir != "" && p.IsTheme() {
		jsPath := filepath.Join(p.InstallDir, "theme.js")
		raw, readErr := os.ReadFile(jsPath)
		// A present-but-unreadable theme.js must NOT skip the scan (that would
		// serve forbidden JS to clients); only a genuinely-absent file is fine.
		if readErr != nil && !os.IsNotExist(readErr) {
			return fmt.Errorf("read theme.js for scanning: %w", readErr)
		}
		if readErr == nil {
			scanErr, scanWarns := themejs.Scan(raw)
			if len(scanWarns) > 0 {
				slog.WarnContext(ctx, "theme JS scanner warnings", slog.String("pluginID", p.ID), slog.Any("warnings", scanWarns))
			}
			if scanErr != nil {
				return fmt.Errorf("plugin blocked: theme.js contains forbidden pattern: %w", scanErr)
			}
		}
	}

	if p.InstallDir != "" && s.fs != nil {
		if cpErr := s.fs.CopySDKStubs(p.Language, p.InstallDir); cpErr != nil {
			slog.WarnContext(ctx, "copy SDK stubs", slog.String("pluginID", p.ID), slog.Any("err", cpErr))
		}
	}

	if p.InstallDir != "" && os.Getuid() == 0 && s.fs != nil {
		s.fs.ChownPluginDir(p.InstallDir)
	}

	if s.analyzer != nil && p.InstallDir != "" {
		report, analyzeErr := s.analyzer.Analyze(p.InstallDir, p.WebRoot)
		if analyzeErr != nil {
			// Fail-closed: without a clean analysis we cannot grant install -
			// a third-party plugin could ship obfuscated malware that the
			// regex analyzer choked on and we'd otherwise install it blind.
			slog.ErrorContext(ctx, "plugin static analysis failed", slog.String("pluginID", p.ID), slog.Any("err", analyzeErr))
			return fmt.Errorf("plugin blocked: static analysis failed: %w", analyzeErr)
		}
		if !report.Safe {
			return &PluginBlockedError{Issues: report.Issues, Categories: report.Categories}
		}
		// Escalate specific WARNING categories to blocking. These patterns are
		// dangerous enough that admin opt-in (like native-binary) or a source
		// fix is required before install. Categories not listed here remain
		// non-blocking warnings persisted for admin review post-install.
		if err := s.checkEscalatedWarnings(ctx, p, report); err != nil {
			return err
		}
		// Native binaries and archives bypass the static analyzer (they
		// are opaque blobs). Block installation by default - the manifest
		// must explicitly request the `native-binary` permission so the
		// admin sees and accepts the elevated risk at preview time.
		if !slices.Contains(p.Permissions, nativeBinaryPerm) {
			if report.HasCategory("compiled_binary") || report.HasCategory("embedded_archive") {
				return fmt.Errorf("plugin blocked: bundles compiled binaries or archives - declare permission %q in manifest to opt in", nativeBinaryPerm)
			}
		}
		// hardware.gpu is the highest-risk permission - append a loud, specific
		// CRITICAL warning so the admin sees it at preview alongside the analyzer
		// findings and consents to the GPU residual risk deliberately.
		if slices.Contains(p.Permissions, gpuPermission) {
			report.Issues = append(report.Issues, gpuPermissionWarning)
			slog.WarnContext(ctx, "plugin declares hardware.gpu (highest-risk permission - render node grant)",
				slog.String("pluginID", p.ID))
		}
		// Persist non-blocking warnings so the admin can audit them
		// post-install (compiled binaries, symlinks, base64 blobs,
		// suspicious URLs). UI exposes these on plugin detail.
		p.AnalysisReport = report
	}
	return nil
}

// swapAndPersistPlugin promotes a validated staging dir over the live
// version on upgrade, then persists the plugin row (Update in place on
// upgrade to keep the FK-referenced id stable and avoid cascade-deleting
// plugin_data, or Create for a fresh install). The returned swapped is true
// as soon as the staging dir has been swapped into finalDir, even if the
// subsequent persist fails - the caller's deferred cleanup must not touch a
// dir that has already been swapped into place.
func (s *InstallerService) swapAndPersistPlugin(ctx context.Context, p *plugin.Plugin, existing *plugin.Plugin, stagingDir, finalDir string) (swapped bool, err error) {
	// All validation on the staging dir passed: swap it over the live version
	// (upgrade only). Any failure before this point left the live version fully
	// intact - files, DB row and config all untouched.
	if existing != nil {
		if err := s.swapUpgradeIntoPlace(ctx, existing, stagingDir, finalDir); err != nil {
			return false, fmt.Errorf("in-place upgrade: %w", err)
		}
		p.InstallDir = finalDir
		swapped = true
	}

	if existing != nil {
		if err := s.plugins.Update(ctx, p); err != nil {
			return swapped, fmt.Errorf("InstallerService.InstallPlugin: %w", err)
		}
	} else if err := s.plugins.Create(ctx, p); err != nil {
		return swapped, fmt.Errorf("InstallerService.InstallPlugin: %w", err)
	}
	return swapped, nil
}

// swapUpgradeIntoPlace atomically promotes a validated, staged upgrade over the
// live version: it stops the old runner, drops cached permissions, removes the
// old install dir and renames the staging dir into finalDir. It deliberately
// leaves the DB row (and its plugin_data, which FK-cascades on plugins.id ON
// DELETE) intact - the caller reuses the same id and Updates the row, so config
// survives. Called only after every validation on the staging dir has passed, so
// a failed upgrade never reaches here and the live version stays untouched.
func (s *InstallerService) swapUpgradeIntoPlace(ctx context.Context, existing *plugin.Plugin, stagingDir, finalDir string) error {
	if existing.Status == plugin.Running {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		stopErr := s.runner.Stop(stopCtx, existing.ID)
		cancel()
		if stopErr != nil {
			slog.WarnContext(ctx, "stop old plugin before upgrade failed, killing",
				slog.String("pluginID", existing.ID), slog.Any("err", stopErr))
			if killErr := s.runner.Kill(existing.ID); killErr != nil {
				slog.WarnContext(ctx, "kill old plugin before upgrade failed",
					slog.String("pluginID", existing.ID), slog.Any("err", killErr))
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	if s.permChecker != nil {
		s.permChecker.InvalidatePermissions(existing.ID)
	}
	if s.fs == nil {
		return nil
	}
	// Remove the old dir, retrying the transient busy-file window (bwrap
	// unmount), then rename staging into place.
	rmErr := s.fs.RemoveAll(finalDir)
	for i := 0; rmErr != nil && i < len(uninstallRetryDelays); i++ {
		time.Sleep(uninstallRetryDelays[i])
		rmErr = s.fs.RemoveAll(finalDir)
	}
	if rmErr != nil {
		return fmt.Errorf("remove old plugin dir: %w", rmErr)
	}
	if err := s.fs.Rename(stagingDir, finalDir); err != nil {
		return fmt.Errorf("swap upgraded plugin into place: %w", err)
	}
	return nil
}

// startPluginAndUpdateStatus starts the runner (non-theme, enabled plugins
// only) and persists the resulting status. It returns true when starting
// failed - the plugin is still considered successfully installed (status
// Error, not a rollback) and the caller must return early, skipping the
// "installed" event publish.
func (s *InstallerService) startPluginAndUpdateStatus(ctx context.Context, p *plugin.Plugin) (startFailed bool) {
	switch {
	case !p.IsTheme() && p.Enabled:
		if err := s.runner.Start(ctx, p); err != nil {
			p.Status = plugin.Error
			p.ErrorMessage = err.Error()
			if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
				slog.WarnContext(ctx, "update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
			}
			return true
		}
		if s.permChecker != nil {
			s.permChecker.LoadPermissions(p.ID, p.Permissions)
		}
		p.Status = plugin.Initializing
	case !p.IsTheme():
		// Upgrading a plugin the admin had disabled: keep it stopped rather than
		// silently re-starting it on update.
		p.Status = plugin.Stopped
	default:
		p.Status = plugin.Running
	}
	if err := s.plugins.Update(ctx, p); err != nil {
		slog.WarnContext(ctx, "update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}
	return false
}

// checkEscalatedWarnings blocks installation when the analysis report contains
// WARNING-severity findings in categories whose risk is high enough to require
// either a source fix or explicit opt-in, not just post-install admin review.
//
// Escalated categories:
//   - "deserialization": yaml.load without Loader - replace with yaml.safe_load
//   - "obfuscation" (warning-level patterns): str_rot13, chr() chains - code obfuscation
//   - "native_code" (Go unsafe.*): allowed when the "native-binary" permission is declared
func (s *InstallerService) checkEscalatedWarnings(_ context.Context, p *plugin.Plugin, report *plugin.AnalysisReport) error {
	blockAndClean := func(msg string) error {
		if s.fs != nil {
			s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
		}
		return fmt.Errorf("plugin blocked: %s", msg)
	}

	if report.HasWarningCategory("deserialization") {
		return blockAndClean("unsafe deserialization patterns detected (e.g. yaml.load without Loader) - use safe alternatives such as yaml.safe_load()")
	}
	if report.HasWarningCategory("obfuscation") {
		return blockAndClean("code obfuscation patterns detected (str_rot13, chr() concatenation) - remove obfuscated constructs before installing")
	}
	// native_code warnings (Go unsafe.*) require explicit native-binary opt-in
	// because unsafe code bypasses Go memory safety guarantees.
	if report.HasWarningCategory("native_code") && !slices.Contains(p.Permissions, nativeBinaryPerm) {
		return blockAndClean(fmt.Sprintf("native/unsafe code patterns detected - declare permission %q in manifest to opt in", nativeBinaryPerm))
	}
	return nil
}
