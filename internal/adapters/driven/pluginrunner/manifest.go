package pluginrunner

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/evey-project/evey/internal/domain/plugin"
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

	// Migrate ui_widgets fallback: if frontend is empty but ui_widgets provided, copy over.
	if len(m.Frontend) == 0 && len(m.UIWidgets) > 0 {
		m.Frontend = m.UIWidgets
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

	return nil
}

// validateExternalFetch checks that every entry in the list is a plain
// hostname or a hostname with an HTTPS scheme — no wildcards, no paths, no
// bare IP addresses (those can't be meaningfully whitelisted in /etc/hosts).
func validateExternalFetch(entries []string) error {
	for _, entry := range entries {
		if strings.HasPrefix(entry, "*.") || entry == "*" {
			return fmt.Errorf("manifest: external_fetch entry %q is a wildcard — use explicit hostnames", entry)
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
		// reject plain IP addresses — hostnames only
		if net := strings.ReplaceAll(host, ".", ""); strings.Trim(net, "0123456789") == "" {
			return fmt.Errorf("manifest: external_fetch entry %q must be a hostname, not an IP address", entry)
		}
	}
	return nil
}
