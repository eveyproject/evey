package pluginrunner

import (
	"archive/zip"
	"fmt"
	"io"

	"github.com/gravity-zero/evey/internal/domain/plugin"
	"gopkg.in/yaml.v3"
)

type YAMLManifestParser struct{}

func NewManifestParser() *YAMLManifestParser {
	return &YAMLManifestParser{}
}

func (p *YAMLManifestParser) Parse(zipPath string) (*plugin.PluginManifest, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

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

	data, err := io.ReadAll(rc)
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

func validateManifest(m *plugin.PluginManifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
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
