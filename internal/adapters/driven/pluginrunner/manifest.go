package pluginrunner

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"gopkg.in/yaml.v3"
)

var semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?$`)

type YAMLManifestParser struct{}

func NewManifestParser() *YAMLManifestParser {
	return &YAMLManifestParser{}
}

func (p *YAMLManifestParser) Parse(zipPath string) (*plugin.PluginManifest, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close() //nolint:errcheck

	var manifestFile *zip.File
	for _, f := range r.File {
		if f.Name == "plugin.yaml" || f.Name == "plugin.yml" {
			manifestFile = f
			break
		}
	}
	if manifestFile == nil {
		return nil, errors.New("plugin.yaml not found in archive")
	}

	rc, err := manifestFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, 1<<20)) // 1MB max manifest
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest plugin.PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	if err := validateManifest(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func (p *YAMLManifestParser) ParseDir(dir string) (*plugin.PluginManifest, error) {
	var data []byte
	var readErr error
	for _, name := range []string{"plugin.yaml", "plugin.yml"} {
		f, fErr := os.Open(filepath.Join(dir, name))
		if fErr != nil {
			readErr = fErr
			continue
		}
		data, readErr = io.ReadAll(io.LimitReader(f, 1<<20))
		f.Close()
		if readErr == nil {
			break
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("read manifest from dir: %w", readErr)
	}

	var manifest plugin.PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := validateManifest(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateManifest(m *plugin.PluginManifest) error {
	if m.Name == "" {
		return errors.New("manifest: name is required")
	}
	if m.Version == "" {
		return errors.New("manifest: version is required")
	}
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("manifest: version %q is not valid semver (expected X.Y.Z or X.Y.Z-label)", m.Version)
	}

	if m.Kind == "" {
		return errors.New("manifest: kind is required (use 'plugin' or 'theme')")
	}

	if m.Kind == plugin.KindTheme {
		// Themes are front-end only: no runtime, no entrypoint, no permissions.
		if m.Language != "" {
			return errors.New("manifest: theme must not have a language")
		}
		if m.Entrypoint != "" {
			return errors.New("manifest: theme must not have an entrypoint")
		}
		if len(m.Permissions) > 0 {
			return errors.New("manifest: theme must not declare permissions")
		}
		if len(m.Capabilities) > 0 {
			return errors.New("manifest: theme must not declare capabilities")
		}
	} else {
		// kind == plugin or empty (backward compat: kind empty treated as plugin).
		if m.Entrypoint == "" {
			return errors.New("manifest: entrypoint is required")
		}
		if m.Language == "" {
			return errors.New("manifest: language is required")
		}

		for _, cap := range m.Capabilities {
			if strings.HasPrefix(cap, "ui.") {
				return fmt.Errorf("manifest: unknown capability %q", cap)
			}
			if !KnownCapabilities[cap] {
				return fmt.Errorf("manifest: unknown capability %q", cap)
			}
		}
	}

	for _, perm := range m.Permissions {
		if !KnownPermissions[perm] {
			return fmt.Errorf("manifest: unknown permission %q", perm)
		}
	}

	if err := validateExternalFetch(m.ExternalFetch); err != nil {
		return err
	}

	if slices.Contains(m.Permissions, "network") && len(m.ExternalFetch) == 0 {
		return errors.New("manifest: network permission requires non-empty external_fetch")
	}

	// web_root is enforced when frontend modules are declared. Domain layer
	// already validates the path syntax + that every src is under web_root;
	// here we just emit a soft warning when the author exposes the entire
	// plugin directory by setting web_root to ".".
	if len(m.Frontend) > 0 && filepath.Clean(m.WebRoot) == "." {
		slog.Warn("manifest web_root=\".\" exposes the entire plugin directory; consider moving frontend assets into a subdir",
			slog.String("plugin", m.Name))
	}

	return nil
}

// validateExternalFetch checks that every entry in the list is a plain
// hostname or a hostname with an HTTPS scheme - no wildcards, no paths, no
// bare IP addresses (those can't be meaningfully whitelisted in /etc/hosts).
func validateExternalFetch(entries []string) error {
	for _, entry := range entries {
		if strings.HasPrefix(entry, "*.") || entry == "*" {
			return fmt.Errorf("manifest: external_fetch entry %q is a wildcard - use explicit hostnames", entry)
		}
		host := entry
		if strings.Contains(entry, "://") {
			u, err := url.Parse(entry)
			if err != nil {
				return fmt.Errorf("manifest: external_fetch entry %q is not a valid URL: %w", entry, err)
			}
			host = u.Hostname()
		}
		if host == "" {
			return fmt.Errorf("manifest: external_fetch entry %q has no resolvable hostname", entry)
		}
		// Reject plain IP addresses - hostnames only. ParseIP covers both
		// families: the previous "strip dots, expect digits" test only ever
		// matched IPv4, so every IPv6 literal ("::1", "2001:db8::1", and the
		// bracketed URL forms) passed a gate that claims to allow no IPs.
		// The SSRF dialer refuses to connect to loopback/reserved addresses
		// regardless, so this is the declaration matching its own promise
		// rather than the last line of defence.
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
			return fmt.Errorf("manifest: external_fetch entry %q must be a hostname, not an IP address", entry)
		}
	}
	return nil
}
