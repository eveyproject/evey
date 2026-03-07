package pluginrunner

import (
	"archive/zip"
	"os"
	"path/filepath"
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
	assert.Error(t, err)
}
