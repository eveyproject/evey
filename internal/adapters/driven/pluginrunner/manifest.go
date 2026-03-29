package pluginrunner

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gravity-zero/evey/internal/domain/plugin"
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
		return nil, fmt.Errorf("plugin.yaml not found in archive")
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
		return fmt.Errorf("manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
	}
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("manifest: version %q is not valid semver (expected X.Y.Z or X.Y.Z-label)", m.Version)
	}
	if m.Entrypoint == "" {
		return fmt.Errorf("manifest: entrypoint is required")
	}
	if m.Language == "" {
		return fmt.Errorf("manifest: language is required")
	}

	for _, cap := range m.Capabilities {
		if !KnownCapabilities[cap] {
			return fmt.Errorf("manifest: unknown capability %q", cap)
		}
	}

	for _, perm := range m.Permissions {
		if !KnownPermissions[perm] {
			return fmt.Errorf("manifest: unknown permission %q", perm)
		}
	}

	return nil
}
