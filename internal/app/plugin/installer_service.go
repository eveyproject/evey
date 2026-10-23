package plugin

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/ssrf"
)

// installer_service.go holds the InstallerService type, its wiring (options +
// constructor) and the default filesystem lister. The behaviour lives in
// cohesive sibling files:
//   - install.go   the install/upgrade write pipeline + static analysis
//   - preview.go   read-only manifest/diff previews
//   - update.go    apply-update + permission-escalation guard
//   - uninstall.go removal + orphan-dir cleanup
//   - semver.go    hashing / semver / set helpers

type InstallerService struct {
	plugins      plugin.PluginRepository
	runner       plugin.PluginRunner
	parser       plugin.ManifestParser
	events       shared.EventBus
	installer    plugin.PluginInstaller
	analyzer     plugin.PluginAnalyzer
	sigVerifier  plugin.SignatureVerifier
	publishers   *TrustedPublisherService
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

// InstallerWithTrustedPublishers wires the trusted-publisher keyring the admin
// RPCs manage. Without it those RPCs report the feature as unavailable.
func InstallerWithTrustedPublishers(tp *TrustedPublisherService) InstallerOption {
	return func(s *InstallerService) { s.publishers = tp }
}

// Publishers returns the trusted-publisher service, nil when not wired.
func (s *InstallerService) Publishers() *TrustedPublisherService { return s.publishers }

// InstallerWithRuntimeChecker wires a host-runtime detector. When set, the
// installer refuses any plugin whose declared language is not available
// on the host, with an actionable message naming the supported runtimes.
// When unset (nil), the check is skipped (preserves existing behaviour for
// tests + callers that opt out).
func InstallerWithRuntimeChecker(c plugin.RuntimeAvailabilityChecker) InstallerOption {
	return func(s *InstallerService) { s.runtimes = c }
}

// InstallerWithHTTPClient replaces the default ssrf-safe HTTP client used
// to fetch remote plugin ZIPs. Intended for tests that need to download
// from a loopback httptest server - production should not bypass SSRF.
func InstallerWithHTTPClient(c *http.Client) InstallerOption {
	return func(s *InstallerService) { s.httpClient = c }
}

// defaultListDirs is the production implementation of listDirsFn - backed
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
