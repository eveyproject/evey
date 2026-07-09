package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/ssrf"
	"github.com/evey-project/evey/internal/pkg/themejs"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// uninstallRetryDelays are the back-off intervals used between RemoveAll
// attempts during uninstall. Three attempts total (initial + two retries).
var uninstallRetryDelays = [2]time.Duration{500 * time.Millisecond, 2 * time.Second}

// downloadIfURL detects if source is an HTTP(S) URL, downloads it into a
// temp file with a 100 MB limit and returns the local path + cleanup func.
// The fragment "#sha256:<hex>" is parsed as the expected hash if present and
// stripped from the URL before the download.
// For local paths, returns source unchanged + noop cleanup + "" hash.
func (s *InstallerService) downloadIfURL(ctx context.Context, source string) (localPath, expectedHash string, cleanup func(), err error) {
	cleanup = func() {}

	// Parse optional "#sha256:<hex>" fragment.
	rawURL, fragment, _ := strings.Cut(source, "#")
	if strings.HasPrefix(fragment, "sha256:") {
		expectedHash = strings.TrimPrefix(fragment, "sha256:")
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

// nativeBinaryPerm is the manifest permission a plugin must declare to
// be allowed to ship compiled binaries or embedded archives. Without it
// the installer rejects any plugin that the static analyzer flagged
// with `compiled_binary` or `embedded_archive` categories — opaque blobs
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
const gpuPermissionWarning = "[CRITICAL] permission hardware.gpu — grants the GPU render node: a kernel-driver attack surface and a GPU-memory boundary SHARED with the transcoder (cross-process memory leak possible on AMD/Apple/Qualcomm GPUs; a hung GPU can stall transcoding). Grant ONLY to first-party / trusted plugins; never to an untrusted source. See docs/plugins.md."

// ErrPluginPermissionEscalation is returned when an update would silently
// grant the plugin new permissions, capabilities, frontend slots, or
// external_fetch entries that were not present in the installed version.
// The caller (admin handler) must pass WithAllowPermissionEscalation(ctx)
// after an explicit admin acknowledgment to proceed.
var ErrPluginPermissionEscalation = errors.New("plugin update requires permission escalation")

// PluginBlockedError is returned by InstallPlugin when the static analyzer
// rejects a plugin for CRITICAL security findings. It carries the structured
// data the http layer needs: the pre-formatted Issues strings (kept for
// backwards-compat with the admin UI which already renders them) AND the
// deduplicated Categories list (machine-readable enums like "shell_exec",
// "sandbox_escape", etc.) which the security-spec test asserts against.
//
// Other "plugin blocked: ..." returns (analyzer-failed, theme.js scanner,
// native-binary opt-in, escalated warnings) do NOT use this type because they
// don't have a per-finding category list — the http handler falls back to its
// existing string-parsing path for those.
type PluginBlockedError struct {
	Issues     []string
	Categories []string
}

func (e *PluginBlockedError) Error() string {
	return fmt.Sprintf("plugin blocked: critical security issues found: %v", e.Issues)
}

type permEscalationCtxKey struct{}

// WithAllowPermissionEscalation returns a child context that authorizes an
// update to proceed even when it adds new permissions/capabilities/slots/hosts.
// Call this only after the admin has explicitly acknowledged the diff.
func WithAllowPermissionEscalation(ctx context.Context) context.Context {
	return context.WithValue(ctx, permEscalationCtxKey{}, true)
}

func allowPermissionEscalation(ctx context.Context) bool {
	v, _ := ctx.Value(permEscalationCtxKey{}).(bool)
	return v
}

type InstallerService struct {
	plugins      plugin.PluginRepository
	runner       plugin.PluginRunner
	parser       plugin.ManifestParser
	events       shared.EventBus
	installer    plugin.PluginInstaller
	analyzer     plugin.PluginAnalyzer
	sigVerifier  plugin.SignatureVerifier
	runtimes     plugin.RuntimeAvailabilityChecker
	fs           plugin.PluginFileSystem
	cssSanitizer plugin.CSSSanitizer
	permChecker  *PermissionChecker
	pluginDir    string
	installMu    sync.Mutex
	// httpClient downloads remote plugin ZIPs. Defaults to an ssrf-safe
	// client; tests inject http.DefaultClient via InstallerWithHTTPClient
	// to hit loopback (httptest.Server) without the SSRF guard firing.
	httpClient *http.Client
	// listDirsFn lists names of plg-* subdirectories under a base dir.
	// Defaults to an os.ReadDir-based implementation; injectable for tests.
	listDirsFn func(baseDir string) ([]string, error)
}

type InstallerOption func(*InstallerService)

func InstallerWithInstaller(installer plugin.PluginInstaller) InstallerOption {
	return func(s *InstallerService) { s.installer = installer }
}

func InstallerWithAnalyzer(analyzer plugin.PluginAnalyzer) InstallerOption {
	return func(s *InstallerService) { s.analyzer = analyzer }
}

func InstallerWithPermissionChecker(pc *PermissionChecker) InstallerOption {
	return func(s *InstallerService) { s.permChecker = pc }
}

func InstallerWithPluginDir(dir string) InstallerOption {
	return func(s *InstallerService) { s.pluginDir = dir }
}

func InstallerWithFileSystem(fs plugin.PluginFileSystem) InstallerOption {
	return func(s *InstallerService) { s.fs = fs }
}

func InstallerWithCSSSanitizer(cs plugin.CSSSanitizer) InstallerOption {
	return func(s *InstallerService) { s.cssSanitizer = cs }
}

func InstallerWithSignatureVerifier(v plugin.SignatureVerifier) InstallerOption {
	return func(s *InstallerService) { s.sigVerifier = v }
}

// InstallerWithRuntimeChecker wires a host-runtime detector. When set, the
// installer refuses any plugin whose declared language is not available
// on the host, with an actionable message naming the supported runtimes.
// When unset (nil), the check is skipped (preserves existing behaviour for
// tests + callers that opt out).
func InstallerWithRuntimeChecker(c plugin.RuntimeAvailabilityChecker) InstallerOption {
	return func(s *InstallerService) { s.runtimes = c }
}

// InstallerWithListDirsFn replaces the default os.ReadDir-based directory
// enumerator. Intended for tests only — production code never needs this.
func InstallerWithListDirsFn(fn func(baseDir string) ([]string, error)) InstallerOption {
	return func(s *InstallerService) { s.listDirsFn = fn }
}

// InstallerWithHTTPClient replaces the default ssrf-safe HTTP client used
// to fetch remote plugin ZIPs. Intended for tests that need to download
// from a loopback httptest server — production should not bypass SSRF.
func InstallerWithHTTPClient(c *http.Client) InstallerOption {
	return func(s *InstallerService) { s.httpClient = c }
}

// defaultListDirs is the production implementation of listDirsFn — backed
// by os.ReadDir, returns only the names of subdirectories.
func defaultListDirs(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func NewInstallerService(
	plugins plugin.PluginRepository,
	runner plugin.PluginRunner,
	parser plugin.ManifestParser,
	events shared.EventBus,
	opts ...InstallerOption,
) *InstallerService {
	s := &InstallerService{
		plugins:   plugins,
		runner:    runner,
		parser:    parser,
		events:    events,
		pluginDir: defaultPluginBaseDir,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.httpClient == nil {
		s.httpClient = ssrf.SafeHTTPClient(5 * time.Minute)
	}
	return s
}

// PluginPreview holds manifest info + analysis without installing.
type PluginPreview struct {
	Name         string
	Version      string
	Description  string
	Author       string
	Language     string
	Entrypoint   string
	Permissions  []string
	Capabilities []string
	Analysis     *plugin.AnalysisReport
	SourceHash   string
}

// PluginUpdatePreview holds the diff between an installed plugin and a
// candidate update ZIP. No state is mutated when building this struct.
type PluginUpdatePreview struct {
	CurrentVersion   string
	CandidateVersion string

	PermissionsDiff   PermissionDiff
	CapabilitiesDiff  PermissionDiff
	SlotsDiff         PermissionDiff
	ExternalFetchDiff PermissionDiff

	// NewFindings are analysis findings present in the candidate but not
	// critical-blocking (those would prevent the preview from being built).
	NewFindings []plugin.PluginFinding

	// SignatureStatus is the cosign result on the candidate. Empty string
	// when sig verification is disabled.
	SignatureStatus string

	// RequiresAck is true when any permission/capability/slot/host was added.
	RequiresAck bool

	// SourceHash is the SHA-256 of the candidate ZIP. Must be passed back
	// in ApplyPluginUpdateRequest.AcknowledgedHash to prevent TOCTOU.
	SourceHash string
}

func (s *InstallerService) InstallPlugin(ctx context.Context, source string) (*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.InstallPlugin")
	defer span.End()
	s.installMu.Lock()
	defer s.installMu.Unlock()

	localPath, expectedHash, cleanupDL, dlErr := s.downloadIfURL(ctx, source)
	defer cleanupDL()
	if dlErr != nil {
		return nil, fmt.Errorf("resolve plugin source: %w", dlErr)
	}
	zipPath := localPath

	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Fail fast if the host image lacks the declared runtime. Themes have no
	// backend language and bypass this check. The checker is optional —
	// callers that don't wire it (legacy tests) keep the previous behaviour.
	if s.runtimes != nil && manifest.Kind != plugin.KindTheme && manifest.Language != "" {
		if !s.runtimes.IsAvailable(manifest.Language) {
			return nil, fmt.Errorf(
				"plugin requires %q runtime which is not installed in this image; available runtimes: %v — install the matching image variant (e.g. evey:beta-nodejs for node, evey:beta-full for all runtimes)",
				manifest.Language, s.runtimes.Available(),
			)
		}
	}

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
		slog.Info("upgrading plugin in place", slog.String("name", manifest.Name),
			slog.String("id", existing.ID), slog.String("from", existing.Version), slog.String("to", manifest.Version))
	}

	sourceHash, hashErr := hashFile(zipPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash plugin file: %w", hashErr)
	}
	if expectedHash != "" && sourceHash != expectedHash {
		return nil, fmt.Errorf("plugin hash mismatch: expected %s, got %s", expectedHash, sourceHash)
	}

	// Cosign signature verification — policy controls whether failure blocks.
	var sigStatus *plugin.SignatureStatus
	if s.sigVerifier != nil {
		sigStatus, err = s.sigVerifier.VerifySignature(ctx, zipPath, "")
		if err != nil {
			// Only reached when policy is "enforce" and verification failed.
			return nil, fmt.Errorf("plugin blocked: %w", err)
		}
	}

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

	// Defense-in-depth: a kind:plugin without a backend language is nonsensical —
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

	// The plugin's canonical dir. On upgrade we extract + validate into a sibling
	// staging dir and only swap it here on success, so a failed upgrade never
	// touches the live version. On a fresh install the staging dir IS the final
	// dir (nothing to protect).
	finalDir := filepath.Join(s.pluginDir, p.ID)
	stagingDir := finalDir
	if existing != nil {
		stagingDir = finalDir + ".upgrade"
	}
	if s.installer != nil {
		// Clear any leftover staging dir from a previously-interrupted upgrade.
		if stagingDir != finalDir && s.fs != nil {
			s.fs.RemoveAll(stagingDir) //nolint:errcheck
		}
		if err := s.installer.Extract(zipPath, stagingDir); err != nil {
			return nil, fmt.Errorf("extract plugin: %w", err)
		}
		p.InstallDir = stagingDir
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

	if p.InstallDir != "" && p.IsTheme() && s.cssSanitizer != nil {
		themePath := filepath.Join(p.InstallDir, "theme.css")
		raw, readErr := os.ReadFile(themePath)
		// A present-but-unreadable theme.css must NOT skip sanitization (that
		// would serve it raw); only a genuinely-absent file is fine to skip.
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read theme.css for sanitization: %w", readErr)
		}
		if readErr == nil {
			sanitized, warns := s.cssSanitizer.SanitizeCSS(raw)
			if len(warns) > 0 {
				slog.Warn("css sanitizer cleaned theme at install", slog.String("pluginID", p.ID), slog.Any("warnings", warns))
				// The write-back is the only sanitization barrier — theme_asset
				// serves theme.css raw (and pre-auth on the login page). If it
				// fails, fail the install so the deferred cleanup removes the dir
				// rather than leaving the dangerous original on disk.
				if writeErr := os.WriteFile(themePath, sanitized, 0o644); writeErr != nil {
					return nil, fmt.Errorf("persist sanitized theme.css: %w", writeErr)
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
			return nil, fmt.Errorf("read theme.js for scanning: %w", readErr)
		}
		if readErr == nil {
			scanErr, scanWarns := themejs.Scan(raw)
			if len(scanWarns) > 0 {
				slog.Warn("theme JS scanner warnings", slog.String("pluginID", p.ID), slog.Any("warnings", scanWarns))
			}
			if scanErr != nil {
				return nil, fmt.Errorf("plugin blocked: theme.js contains forbidden pattern: %w", scanErr)
			}
		}
	}

	if p.InstallDir != "" && s.fs != nil {
		if cpErr := s.fs.CopySDKStubs(p.Language, p.InstallDir); cpErr != nil {
			slog.Warn("copy SDK stubs", slog.String("pluginID", p.ID), slog.Any("err", cpErr))
		}
	}

	if p.InstallDir != "" && os.Getuid() == 0 && s.fs != nil {
		s.fs.ChownPluginDir(p.InstallDir)
	}

	if s.analyzer != nil && p.InstallDir != "" {
		report, analyzeErr := s.analyzer.Analyze(p.InstallDir, p.WebRoot)
		if analyzeErr != nil {
			// Fail-closed: without a clean analysis we cannot grant install —
			// a third-party plugin could ship obfuscated malware that the
			// regex analyzer choked on and we'd otherwise install it blind.
			slog.Error("plugin static analysis failed", slog.String("pluginID", p.ID), slog.Any("err", analyzeErr))
			return nil, fmt.Errorf("plugin blocked: static analysis failed: %w", analyzeErr)
		}
		if !report.Safe {
			return nil, &PluginBlockedError{Issues: report.Issues, Categories: report.Categories}
		}
		// Escalate specific WARNING categories to blocking. These patterns are
		// dangerous enough that admin opt-in (like native-binary) or a source
		// fix is required before install. Categories not listed here remain
		// non-blocking warnings persisted for admin review post-install.
		if err := s.checkEscalatedWarnings(ctx, p, report); err != nil {
			return nil, err
		}
		// Native binaries and archives bypass the static analyzer (they
		// are opaque blobs). Block installation by default — the manifest
		// must explicitly request the `native-binary` permission so the
		// admin sees and accepts the elevated risk at preview time.
		if !slices.Contains(p.Permissions, nativeBinaryPerm) {
			if report.HasCategory("compiled_binary") || report.HasCategory("embedded_archive") {
				return nil, fmt.Errorf("plugin blocked: bundles compiled binaries or archives — declare permission %q in manifest to opt in", nativeBinaryPerm)
			}
		}
		// hardware.gpu is the highest-risk permission — append a loud, specific
		// CRITICAL warning so the admin sees it at preview alongside the analyzer
		// findings and consents to the GPU residual risk deliberately.
		if slices.Contains(p.Permissions, gpuPermission) {
			report.Issues = append(report.Issues, gpuPermissionWarning)
			slog.Warn("plugin declares hardware.gpu (highest-risk permission — render node grant)",
				slog.String("pluginID", p.ID))
		}
		// Persist non-blocking warnings so the admin can audit them
		// post-install (compiled binaries, symlinks, base64 blobs,
		// suspicious URLs). UI exposes these on plugin detail.
		p.AnalysisReport = report
	}

	// All validation on the staging dir passed: swap it over the live version
	// (upgrade only). Any failure before this point left the live version fully
	// intact - files, DB row and config all untouched.
	if existing != nil {
		if err := s.swapUpgradeIntoPlace(ctx, existing, stagingDir, finalDir); err != nil {
			return nil, fmt.Errorf("in-place upgrade: %w", err)
		}
		p.InstallDir = finalDir
		swapped = true
	}

	// Update the existing row in place on upgrade (id reused above) so the
	// FK-referenced id is stable and plugin_data is not cascade-deleted; Create
	// a fresh row only for a new install.
	if existing != nil {
		if err := s.plugins.Update(ctx, p); err != nil {
			return nil, fmt.Errorf("InstallerService.InstallPlugin: %w", err)
		}
	} else if err := s.plugins.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("InstallerService.InstallPlugin: %w", err)
	}

	switch {
	case !p.IsTheme() && p.Enabled:
		if err := s.runner.Start(ctx, p); err != nil {
			p.Status = plugin.Error
			p.ErrorMessage = err.Error()
			if updateErr := s.plugins.Update(ctx, p); updateErr != nil {
				slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", updateErr))
			}
			installOK = true
			return p, nil
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
		slog.Warn("update plugin status failed", slog.String("pluginID", p.ID), slog.Any("err", err))
	}

	evtInstalled := plugin.PluginInstalled{PluginID: p.ID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtInstalled); err != nil {
		slog.Warn("publish event", slog.String("event", evtInstalled.EventName()), slog.Any("err", err))
	}
	installOK = true
	return p, nil
}

func (s *InstallerService) PreviewPlugin(_ context.Context, zipPath string) (*PluginPreview, error) {
	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	sourceHash, hashErr := hashFile(zipPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash plugin file: %w", hashErr)
	}
	preview := &PluginPreview{
		Name:         manifest.Name,
		Version:      manifest.Version,
		Description:  manifest.Description,
		Author:       manifest.Author,
		Language:     manifest.Language,
		Entrypoint:   manifest.Entrypoint,
		Permissions:  manifest.Permissions,
		Capabilities: manifest.Capabilities,
		SourceHash:   sourceHash,
	}
	if s.installer != nil && s.analyzer != nil && s.fs != nil {
		tmpDir, tmpErr := s.fs.MkdirTemp("", "evey-plugin-preview-*")
		if tmpErr == nil {
			defer func() {
				if rmErr := s.fs.RemoveAll(tmpDir); rmErr != nil {
					slog.Warn("remove preview tmpdir", slog.Any("err", rmErr))
				}
			}()
			if extErr := s.installer.Extract(zipPath, tmpDir); extErr == nil {
				report, analyzeErr := s.analyzer.Analyze(tmpDir, manifest.WebRoot)
				if analyzeErr == nil {
					preview.Analysis = report
				}
			}
		}
	}
	return preview, nil
}

// PreviewUpdate builds a PluginUpdatePreview by comparing the currently
// installed plugin to the candidate ZIP. No state is mutated. The returned
// SourceHash must be echoed back in ApplyUpdate as AcknowledgedHash to
// prevent TOCTOU attacks.
func (s *InstallerService) PreviewUpdate(ctx context.Context, pluginID, source string) (*PluginUpdatePreview, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.PreviewUpdate")
	defer span.End()
	existing, err := s.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, ErrPluginNotFound
	}

	localPath, _, cleanupDL, dlErr := s.downloadIfURL(ctx, source)
	defer cleanupDL()
	if dlErr != nil {
		return nil, fmt.Errorf("resolve plugin source: %w", dlErr)
	}

	manifest, parseErr := s.parser.Parse(localPath)
	if parseErr != nil {
		return nil, fmt.Errorf("parse manifest: %w", parseErr)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	sourceHash, hashErr := hashFile(localPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash candidate: %w", hashErr)
	}

	permDiff := DiffPermissions(existing.Permissions, manifest.Permissions)
	capDiff := DiffCapabilities(existing.Capabilities, manifest.Capabilities)
	slotDiff := DiffSlots(existing.Frontend, manifest.Frontend)
	fetchDiff := DiffExternalFetch(existing.ExternalFetch, manifest.ExternalFetch)

	requiresAck := len(permDiff.Added) > 0 || len(capDiff.Added) > 0 ||
		len(slotDiff.Added) > 0 || len(fetchDiff.Added) > 0

	preview := &PluginUpdatePreview{
		CurrentVersion:    existing.Version,
		CandidateVersion:  manifest.Version,
		PermissionsDiff:   permDiff,
		CapabilitiesDiff:  capDiff,
		SlotsDiff:         slotDiff,
		ExternalFetchDiff: fetchDiff,
		RequiresAck:       requiresAck,
		SourceHash:        sourceHash,
	}

	// Run analyzer on candidate in a temp dir (best-effort — errors don't block preview).
	if s.installer != nil && s.analyzer != nil && s.fs != nil {
		tmpDir, tmpErr := s.fs.MkdirTemp("", "evey-plugin-update-preview-*")
		if tmpErr == nil {
			defer func() {
				if rmErr := s.fs.RemoveAll(tmpDir); rmErr != nil {
					slog.Warn("remove update preview tmpdir", slog.Any("err", rmErr))
				}
			}()
			if extErr := s.installer.Extract(localPath, tmpDir); extErr == nil {
				report, analyzeErr := s.analyzer.Analyze(tmpDir, manifest.WebRoot)
				if analyzeErr == nil {
					preview.NewFindings = report.RawFindings
				}
			}
		}
	}

	// Cosign on candidate (best-effort — status recorded, not blocking).
	if s.sigVerifier != nil {
		sigStatus, sigErr := s.sigVerifier.VerifySignature(ctx, localPath, "")
		if sigErr != nil {
			preview.SignatureStatus = "invalid"
		} else if sigStatus != nil && sigStatus.Verified {
			preview.SignatureStatus = "signed"
		} else {
			preview.SignatureStatus = "unsigned"
		}
	}

	return preview, nil
}

// ApplyUpdate applies a previously previewed update. It re-downloads (or re-reads)
// the candidate and re-computes its hash to verify it matches acknowledged_hash,
// preventing TOCTOU. After hash verification it delegates to InstallPlugin with
// WithAllowPermissionEscalation so the admin's explicit ack is honoured.
func (s *InstallerService) ApplyUpdate(ctx context.Context, pluginID, source, acknowledgedHash string) (*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.ApplyUpdate")
	defer span.End()
	// Verify the plugin exists before going further.
	if _, err := s.plugins.GetByID(ctx, pluginID); err != nil {
		return nil, ErrPluginNotFound
	}

	// Re-download/re-read candidate to compute fresh hash (TOCTOU guard).
	localPath, _, cleanupDL, dlErr := s.downloadIfURL(ctx, source)
	defer cleanupDL()
	if dlErr != nil {
		return nil, fmt.Errorf("resolve plugin source: %w", dlErr)
	}

	freshHash, hashErr := hashFile(localPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash candidate: %w", hashErr)
	}

	if freshHash != acknowledgedHash {
		return nil, fmt.Errorf("%w: expected %s got %s", ErrCandidateChanged, acknowledgedHash, freshHash)
	}

	// Permission escalation is explicitly authorised because the admin has
	// reviewed and acknowledged the diff in the preview step.
	ctx = WithAllowPermissionEscalation(ctx)
	return s.InstallPlugin(ctx, source)
}

// UninstallPlugin guarantees a clean removal: any process is stopped (and
// SIGKILL'd if Stop hangs), the install dir is removed (with retries to ride
// out busy-file races during sandbox teardown), the DB row is deleted, and
// permissions/event hooks fire. It always returns nil except when the plugin
// id is unknown — the admin must never see "uninstall failed" because the
// runner refused to die or a file was momentarily locked.
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
			slog.Warn("stop old plugin before upgrade failed, killing",
				slog.String("pluginID", existing.ID), slog.Any("err", stopErr))
			if killErr := s.runner.Kill(existing.ID); killErr != nil {
				slog.Warn("kill old plugin before upgrade failed",
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

func (s *InstallerService) UninstallPlugin(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.UninstallPlugin")
	defer span.End()
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}

	// 1) Stop runner gracefully with a tight deadline. If it hangs or errors,
	//    SIGKILL the whole process group so any child (the plugin process and
	//    its bwrap sandbox) dies with it.
	if p.Status == plugin.Running {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		stopErr := s.runner.Stop(stopCtx, id)
		cancel()
		if stopErr != nil {
			slog.Warn("plugin Stop failed, falling back to Kill",
				slog.String("pluginID", id), slog.Any("err", stopErr))
			if killErr := s.runner.Kill(id); killErr != nil {
				slog.Warn("plugin Kill failed (will continue)",
					slog.String("pluginID", id), slog.Any("err", killErr))
			}
			// Give the kernel a beat to recycle any FDs the plugin held open
			// (sandbox mount, log files, …) before we attempt RemoveAll.
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 2) Drop cached permissions before we hand the FS path to RemoveAll —
	//    permChecker is best-effort and never blocks uninstall.
	if s.permChecker != nil {
		s.permChecker.InvalidatePermissions(p.ID)
	}

	// 3) Wipe the install dir, retrying on transient busy-file errors that
	//    surface when the sandbox isn't fully torn down yet. Three attempts
	//    with exponential backoff (100ms, 500ms, 2s) covers the typical
	//    bwrap unmount window. If it still fails we log and keep going so
	//    the DB row gets cleaned anyway — a future CleanupOrphans pass will
	//    eventually pick up a stuck dir.
	if p.InstallDir != "" && s.fs != nil {
		backoffs := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}
		var lastErr error
		for i, wait := range backoffs {
			if rmErr := s.fs.RemoveAll(p.InstallDir); rmErr == nil {
				lastErr = nil
				break
			} else {
				lastErr = rmErr
				if i < len(backoffs)-1 {
					time.Sleep(wait)
				}
			}
		}
		if lastErr != nil {
			slog.Error("plugin install dir not removed after retries (will be picked up by CleanupOrphans)",
				slog.String("pluginID", id), slog.String("dir", p.InstallDir), slog.Any("err", lastErr))
		}
	}

	// 4) Delete the DB row. If this fails the FS is already gone (or queued
	//    for cleanup); retrying the uninstall later is safe so we just log.
	if err := s.plugins.Delete(ctx, id); err != nil {
		slog.Error("plugin DB delete failed (admin can retry)",
			slog.String("pluginID", id), slog.Any("err", err))
	}

	// 5) Fire the event so subscribers (slot registry, theme cache, …) drop
	//    their references. Best-effort.
	evtUninstalled := plugin.PluginUninstalled{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtUninstalled); err != nil {
		slog.Warn("publish event", slog.String("event", evtUninstalled.EventName()), slog.Any("err", err))
	}
	return nil
}

// CleanupOrphans walks the configured pluginDir and removes any plg-*
// directory that has no matching row in the plugin repository. Designed to
// be called once at server startup so historical leftovers (uninstalls that
// crashed before the FS removal succeeded) get reaped automatically.
// Best-effort: errors are logged, never returned. Returns the number of
// dirs that were removed.
func (s *InstallerService) CleanupOrphans(ctx context.Context) int {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.CleanupOrphans")
	defer span.End()
	if s.fs == nil || s.pluginDir == "" {
		return 0
	}
	listDirs := s.listDirsFn
	if listDirs == nil {
		listDirs = defaultListDirs
	}
	names, err := listDirs(s.pluginDir)
	if err != nil {
		slog.Warn("cleanup orphans: read plugin dir", slog.String("dir", s.pluginDir), slog.Any("err", err))
		return 0
	}

	all, err := s.plugins.List(ctx)
	if err != nil {
		slog.Warn("cleanup orphans: list plugins from db", slog.Any("err", err))
		return 0
	}
	known := make(map[string]struct{}, len(all))
	for _, p := range all {
		known[p.ID] = struct{}{}
	}

	removed := 0
	for _, name := range names {
		if len(name) < 4 || name[:4] != "plg-" {
			continue
		}
		if _, ok := known[name]; ok {
			continue
		}
		full := filepath.Join(s.pluginDir, name)
		if err := s.fs.RemoveAll(full); err != nil {
			slog.Warn("cleanup orphans: remove dir failed",
				slog.String("dir", full), slog.Any("err", err))
			continue
		}
		slog.Info("cleanup orphans: removed orphan plugin dir", slog.String("dir", full))
		removed++
	}
	if removed > 0 {
		slog.Info("cleanup orphans: complete", slog.Int("removed", removed))
	}
	return removed
}

// checkEscalatedWarnings blocks installation when the analysis report contains
// WARNING-severity findings in categories whose risk is high enough to require
// either a source fix or explicit opt-in, not just post-install admin review.
//
// Escalated categories:
//   - "deserialization": yaml.load without Loader — replace with yaml.safe_load
//   - "obfuscation" (warning-level patterns): str_rot13, chr() chains — code obfuscation
//   - "native_code" (Go unsafe.*): allowed when the "native-binary" permission is declared
func (s *InstallerService) checkEscalatedWarnings(_ context.Context, p *plugin.Plugin, report *plugin.AnalysisReport) error {
	blockAndClean := func(msg string) error {
		if s.fs != nil {
			s.fs.RemoveAll(p.InstallDir) //nolint:errcheck
		}
		return fmt.Errorf("plugin blocked: %s", msg)
	}

	if report.HasWarningCategory("deserialization") {
		return blockAndClean("unsafe deserialization patterns detected (e.g. yaml.load without Loader) — use safe alternatives such as yaml.safe_load()")
	}
	if report.HasWarningCategory("obfuscation") {
		return blockAndClean("code obfuscation patterns detected (str_rot13, chr() concatenation) — remove obfuscated constructs before installing")
	}
	// native_code warnings (Go unsafe.*) require explicit native-binary opt-in
	// because unsafe code bypasses Go memory safety guarantees.
	if report.HasWarningCategory("native_code") && !slices.Contains(p.Permissions, nativeBinaryPerm) {
		return blockAndClean(fmt.Sprintf("native/unsafe code patterns detected — declare permission %q in manifest to opt in", nativeBinaryPerm))
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CompareSemver compares two semver strings (X.Y.Z). Returns -1, 0, or 1.
// Pre-release labels are ignored for numeric comparison.
func CompareSemver(a, b string) int {
	pa := parseSemverParts(a)
	pb := parseSemverParts(b)
	for i := range 3 {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

// checkPermissionEscalation returns ErrPluginPermissionEscalation when the
// incoming manifest requests permissions, capabilities, frontend slots, or
// external_fetch entries that are absent from the currently installed plugin.
// Same-or-reduced sets are allowed without escalation.
func checkPermissionEscalation(manifest *plugin.PluginManifest, existing *plugin.Plugin) error {
	existingPerms := sliceToSet(existing.Permissions)
	existingCaps := sliceToSet(existing.Capabilities)
	existingHosts := sliceToSet(existing.ExternalFetch)

	existingSlots := make(map[string]struct{}, len(existing.Frontend))
	for _, f := range existing.Frontend {
		existingSlots[f.Slot] = struct{}{}
	}

	var added []string
	for _, p := range manifest.Permissions {
		if _, ok := existingPerms[p]; !ok {
			added = append(added, "permission:"+p)
		}
	}
	for _, c := range manifest.Capabilities {
		if _, ok := existingCaps[c]; !ok {
			added = append(added, "capability:"+c)
		}
	}
	for _, h := range manifest.ExternalFetch {
		if _, ok := existingHosts[h]; !ok {
			added = append(added, "external_fetch:"+h)
		}
	}
	for _, f := range manifest.Frontend {
		if _, ok := existingSlots[f.Slot]; !ok {
			added = append(added, "slot:"+f.Slot)
		}
	}

	if len(added) > 0 {
		return fmt.Errorf("%w: new entries requested: %s", ErrPluginPermissionEscalation, strings.Join(added, ", "))
	}
	return nil
}

// sliceToSet converts a string slice to a set map for O(1) membership tests.
func sliceToSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

func parseSemverParts(v string) [3]int {
	// strip pre-release label
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := range 3 {
		if i < len(parts) {
			n, _ := strconv.Atoi(parts[i])
			result[i] = n
		}
	}
	return result
}
