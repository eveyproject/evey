package pluginrunner

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
}

func TestParseYmlExtension(t *testing.T) {
	yaml := `
name: test-plugin
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
	huge := "name: big\nversion: \"1.0.0\"\nlanguage: go\nentrypoint: ./plugin\ndescription: " + strings.Repeat("x", 2*1024*1024) + "\n"
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
	valid := []string{"1.0.0", "0.1.0", "10.20.30", "1.0.0-beta.1", "1.0.0-rc.1"}
	parser := NewManifestParser()
	for _, v := range valid {
		yml := "name: test\nversion: \"" + v + "\"\nlanguage: go\nentrypoint: ./plugin\n"
		zipPath := createTestZip(t, "plugin.yaml", yml)
		_, err := parser.Parse(zipPath)
		assert.NoError(t, err, "version %q should be valid", v)
	}
}

func TestParse_InvalidSemver(t *testing.T) {
	invalid := []string{"1.0", "1", "v1.0.0", "1.0.0.0", "abc", "1.0.x", "01.0.0"}
	parser := NewManifestParser()
	for _, v := range invalid {
		yml := "name: test\nversion: \"" + v + "\"\nlanguage: go\nentrypoint: ./plugin\n"
		zipPath := createTestZip(t, "plugin.yaml", yml)
		_, err := parser.Parse(zipPath)
		assert.ErrorContains(t, err, "not valid semver", "version %q should be rejected", v)
	}
}
