package pluginrunner

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestZip(t *testing.T, filename, content string) string {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "plugin.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	fw, err := w.Create(filename)
	require.NoError(t, err)
	_, err = fw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return zipPath
}

func TestParseValidManifest(t *testing.T) {
	yaml := `
name: test-plugin
kind: plugin
version: "1.0.0"
description: A test plugin
author: tester
language: go
entrypoint: ./plugin
capabilities:
  - media.read
permissions:
  - network
  - api.library
external_fetch:
  - api.tmdb.org
min_evey_version: "1.0.0"
`
	zipPath := createTestZip(t, "plugin.yaml", yaml)
	parser := NewManifestParser()

	m, err := parser.Parse(zipPath)
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, "A test plugin", m.Description)
	assert.Equal(t, "tester", m.Author)
	assert.Equal(t, "go", m.Language)
	assert.Equal(t, "./plugin", m.Entrypoint)
	assert.Equal(t, []string{"media.read"}, m.Capabilities)
	assert.Equal(t, []string{"network", "api.library"}, m.Permissions)
	assert.Equal(t, "1.0.0", m.MinEveyVersion)
	assert.Equal(t, plugin.KindPlugin, m.Kind)
	assert.Equal(t, []string{"api.tmdb.org"}, m.ExternalFetch)
}

func TestParseYmlExtension(t *testing.T) {
	yaml := `
name: test-plugin
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
`
	zipPath := createTestZip(t, "plugin.yml", yaml)
	parser := NewManifestParser()

	m, err := parser.Parse(zipPath)
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", m.Name)
}

func TestParseMissingName(t *testing.T) {
	yaml := `
version: "1.0.0"
language: go
entrypoint: ./plugin
`
	zipPath := createTestZip(t, "plugin.yaml", yaml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "name is required")
}

func TestParseMissingVersion(t *testing.T) {
	yaml := `
name: test-plugin
language: go
entrypoint: ./plugin
`
	zipPath := createTestZip(t, "plugin.yaml", yaml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "version is required")
}

func TestParseMissingEntrypoint(t *testing.T) {
	yaml := `
name: test-plugin
kind: plugin
version: "1.0.0"
language: go
`
	zipPath := createTestZip(t, "plugin.yaml", yaml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "entrypoint is required")
}

func TestParseMissingLanguage(t *testing.T) {
	yaml := `
name: test-plugin
kind: plugin
version: "1.0.0"
entrypoint: ./plugin
`
	zipPath := createTestZip(t, "plugin.yaml", yaml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "language is required")
}

func TestParseInvalidCapability(t *testing.T) {
	yaml := `
name: test-plugin
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
capabilities:
  - invalid.cap
`
	zipPath := createTestZip(t, "plugin.yaml", yaml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "unknown capability")
}

func TestParseInvalidPermission(t *testing.T) {
	yaml := `
name: test-plugin
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
permissions:
  - invalid.perm
`
	zipPath := createTestZip(t, "plugin.yaml", yaml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "unknown permission")
}

func TestParseNoManifestInZip(t *testing.T) {
	zipPath := createTestZip(t, "other.txt", "hello")
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "plugin.yaml not found")
}

func TestParseInvalidZipPath(t *testing.T) {
	parser := NewManifestParser()
	_, err := parser.Parse("/nonexistent/path.zip")
	assert.ErrorContains(t, err, "open")
}

func TestParse_PluginYml(t *testing.T) {
	yml := `
name: yml-plugin
kind: plugin
version: "1.0.0"
language: python
entrypoint: python main.py
`
	zipPath := createTestZip(t, "plugin.yml", yml)
	parser := NewManifestParser()

	m, err := parser.Parse(zipPath)
	require.NoError(t, err)
	assert.Equal(t, "yml-plugin", m.Name)
}

func TestParse_ManifestSizeLimit(t *testing.T) {
	huge := "name: big\nkind: plugin\nversion: \"1.0.0\"\nlanguage: go\nentrypoint: ./plugin\ndescription: " + strings.Repeat("x", 2*1024*1024) + "\n"
	zipPath := createTestZip(t, "plugin.yaml", huge)
	parser := NewManifestParser()

	m, err := parser.Parse(zipPath)
	// LimitReader truncates at 1MB, yaml parse may succeed with truncated data or fail
	// Either way it should not crash or return a manifest with the full 2MB string
	if err == nil && len(m.Description) > 1<<20 {
		t.Fatal("manifest should be limited to 1MB read")
	}
}

func TestParse_EmptyName_Fails(t *testing.T) {
	yml := `
name: ""
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "name is required")
}

func TestParse_EmptyEntrypoint_Fails(t *testing.T) {
	yml := `
name: test
kind: plugin
version: "1.0.0"
language: go
entrypoint: ""
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "entrypoint is required")
}

func TestParse_UnknownCapability_Fails(t *testing.T) {
	yml := `
name: test
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
capabilities:
  - totally.fake.cap
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "unknown capability")
}

func TestParse_UnknownPermission_Fails(t *testing.T) {
	yml := `
name: test
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
permissions:
  - root.access
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "unknown permission")
}

func TestParseDir_FallbackToYml(t *testing.T) {
	dir := t.TempDir()
	yml := `
name: dir-yml-plugin
kind: plugin
version: "1.0.0"
language: python
entrypoint: python main.py
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.yml"), []byte(yml), 0644))
	parser := NewManifestParser()

	m, err := parser.ParseDir(dir)
	require.NoError(t, err)
	assert.Equal(t, "dir-yml-plugin", m.Name)
}

func TestParseDir_NotFound(t *testing.T) {
	dir := t.TempDir()
	parser := NewManifestParser()

	_, err := parser.ParseDir(dir)
	assert.ErrorContains(t, err, "read manifest from dir")
}

func TestParse_ValidSemver(t *testing.T) {
	valid := []string{
		"1.0.0", "0.1.0", "10.20.30",
		"1.0.0-alpha", "1.0.0-beta.1", "1.0.0-rc.1",
		"0.0.0", "999.999.999",
		"1.0.0-0.3.7", "1.0.0-x.7.z.92",
	}
	parser := NewManifestParser()
	for _, v := range valid {
		t.Run(v, func(t *testing.T) {
			yml := "name: test\nkind: plugin\nversion: \"" + v + "\"\nlanguage: go\nentrypoint: ./plugin\n"
			zipPath := createTestZip(t, "plugin.yaml", yml)
			_, err := parser.Parse(zipPath)
			assert.NoError(t, err, "version %q should be valid", v)
		})
	}
}

func TestParse_InvalidSemver(t *testing.T) {
	invalid := []string{
		"1.0", "1", "v1.0.0", "1.0.0.0", "abc", "1.0.x",
		"01.0.0", "0.01.0", "0.0.01",
		"", " 1.0.0", "1.0.0 ", "1.0.0-", "1.0.0+build",
	}
	parser := NewManifestParser()
	for _, v := range invalid {
		t.Run("reject_"+v, func(t *testing.T) {
			yml := "name: test\nkind: plugin\nversion: \"" + v + "\"\nlanguage: go\nentrypoint: ./plugin\n"
			zipPath := createTestZip(t, "plugin.yaml", yml)
			_, err := parser.Parse(zipPath)
			if v == "" {
				assert.ErrorContains(t, err, "version is required", "empty version should fail required check")
			} else {
				assert.ErrorContains(t, err, "not valid semver", "version %q should be rejected", v)
			}
		})
	}
}

func TestParse_NetworkWithoutExtfetch_Fails(t *testing.T) {
	yml := `
name: test
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
permissions:
  - network
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "network permission requires")
}

func TestParse_NetworkWithExtfetch_OK(t *testing.T) {
	yml := `
name: test
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
permissions:
  - network
external_fetch:
  - api.tmdb.org
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	m, err := parser.Parse(zipPath)
	require.NoError(t, err)
	assert.Equal(t, []string{"api.tmdb.org"}, m.ExternalFetch)
}

func TestParse_InvalidExtfetch_Fails(t *testing.T) {
	yml := `
name: test
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
external_fetch:
  - "*"
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "wildcard")
}

func TestSemverRegex_Directly(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{"1.0.0", true},
		{"0.0.1", true},
		{"1.0.0-alpha", true},
		{"1.0.0-alpha.1", true},
		{"01.0.0", false},
		{"0.01.0", false},
		{"0.0.01", false},
		{"1.0", false},
		{"v1.0.0", false},
		{"1.0.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := semverRe.MatchString(tt.version)
			assert.Equal(t, tt.valid, got, "semverRe.MatchString(%q)", tt.version)
		})
	}
}

// --- kind-aware tests ---

func TestParseTheme_Valid(t *testing.T) {
	yml := `
name: dark-theme
kind: theme
version: "1.0.0"
description: A dark theme
frontend:
  - slot: player.skin
    src: theme.css
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	m, err := parser.Parse(zipPath)
	require.NoError(t, err)
	assert.Equal(t, plugin.KindTheme, m.Kind)
	assert.Equal(t, "dark-theme", m.Name)
	assert.Empty(t, m.Language)
	assert.Empty(t, m.Entrypoint)
	require.Len(t, m.Frontend, 1)
	assert.Equal(t, "player.skin", m.Frontend[0].Slot)
}

func TestParseTheme_WithEntrypoint_Fails(t *testing.T) {
	yml := `
name: dark-theme
kind: theme
version: "1.0.0"
entrypoint: ./theme
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "theme must not have an entrypoint")
}

func TestParseTheme_WithLanguage_Fails(t *testing.T) {
	yml := `
name: dark-theme
kind: theme
version: "1.0.0"
language: css
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "theme must not have a language")
}

func TestParseTheme_WithPermissions_Fails(t *testing.T) {
	yml := `
name: dark-theme
kind: theme
version: "1.0.0"
permissions:
  - network
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "theme must not declare permissions")
}

func TestParseTheme_WithCapabilities_Fails(t *testing.T) {
	yml := `
name: dark-theme
kind: theme
version: "1.0.0"
capabilities:
  - media.read
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "theme must not declare capabilities")
}

func TestParsePlugin_WithoutEntrypoint_Fails(t *testing.T) {
	yml := `
name: my-plugin
kind: plugin
version: "1.0.0"
language: go
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "entrypoint is required")
}

func TestParsePlugin_WithoutLanguage_Fails(t *testing.T) {
	yml := `
name: my-plugin
kind: plugin
version: "1.0.0"
entrypoint: ./plugin
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "language is required")
}

func TestParsePlugin_UIThemeCapability_Fails(t *testing.T) {
	yml := `
name: my-plugin
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
capabilities:
  - ui.theme
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "unknown capability")
}

func TestParsePlugin_UIExtensionCapability_Fails(t *testing.T) {
	yml := `
name: my-plugin
kind: plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
capabilities:
  - ui.extension
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "unknown capability")
}

func TestParseKindEmpty_Rejected(t *testing.T) {
	// New manifests must declare kind explicitly; migrateManifest (installer layer)
	// handles old manifests before validateManifest is called.
	yml := `
name: old-plugin
version: "1.0.0"
language: go
entrypoint: ./plugin
`
	zipPath := createTestZip(t, "plugin.yaml", yml)
	parser := NewManifestParser()

	_, err := parser.Parse(zipPath)
	assert.ErrorContains(t, err, "kind is required")
}

// The "hostnames only, no bare IPs" rule stripped dots and checked for digits,
// which catches 127.0.0.1 but sails past every IPv6 literal. The SSRF dialer
// still refuses to connect to loopback/reserved ranges, so this was never an
// escape - but the manifest gate should mean what it says rather than lean on
// the layer below.
func TestValidateExternalFetch_RejectsIPLiterals(t *testing.T) {
	for _, entry := range []string{
		"127.0.0.1",
		"192.168.1.10",
		"::1",
		"[::1]",
		"https://[::1]/x",
		"2001:db8::1",
		"https://[2001:db8::1]/",
	} {
		if err := validateExternalFetch([]string{entry}); err == nil {
			t.Errorf("external_fetch %q is an IP literal and must be rejected", entry)
		}
	}
}

func TestValidateExternalFetch_KeepsAcceptingHostnames(t *testing.T) {
	for _, entry := range []string{
		"api.tmdb.org",
		"https://api.themoviedb.org",
		"example.co.uk",
	} {
		if err := validateExternalFetch([]string{entry}); err != nil {
			t.Errorf("external_fetch %q is a legitimate hostname, got %v", entry, err)
		}
	}
}
