package pluginrunner

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeLauncherZip builds a plugin archive holding the named files at its root.
func writeLauncherZip(t *testing.T, names ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "plugin.zip")
	f, err := os.Create(p)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	for _, n := range append([]string{"plugin.yaml"}, names...) {
		w, err := zw.Create(n)
		require.NoError(t, err)
		_, err = w.Write([]byte("x"))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return p
}

// A launcher the image has is accepted whatever the manifest language: no
// language list gates it. "sh" stands for java or dotnet, present on every host.
func TestCheckEntrypoint_LauncherOnPathAnyLanguage(t *testing.T) {
	r := NewRuntimeAvailability("/data/plugins")
	zipPath := writeLauncherZip(t, "plugin.jar")
	for _, lang := range []string{"java", "dotnet", "cobol-85", ""} {
		assert.NoError(t, r.CheckEntrypoint(zipPath, lang, "sh -c true"), lang)
	}
}

func TestCheckEntrypoint_LauncherMissingRefused(t *testing.T) {
	r := NewRuntimeAvailability("/data/plugins")
	err := r.CheckEntrypoint(writeLauncherZip(t, "plugin.jar"), "java", "evey-no-such-jre -jar plugin.jar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"evey-no-such-jre"`)
}

// A binary the plugin ships needs nothing from the image.
func TestCheckEntrypoint_ShippedBinary(t *testing.T) {
	r := NewRuntimeAvailability("/data/plugins")
	zipPath := writeLauncherZip(t, "plugin", "bin/server")
	assert.NoError(t, r.CheckEntrypoint(zipPath, "go", "plugin --flag"))
	assert.NoError(t, r.CheckEntrypoint(zipPath, "rust", "./bin/server"))
	assert.NoError(t, r.CheckEntrypoint(zipPath, "", "plugin"))
}

// A shipped script runs through the interpreter its language names.
func TestCheckEntrypoint_ShippedScriptNeedsItsInterpreter(t *testing.T) {
	saved := shebangInterpreters["python"]
	t.Cleanup(func() { shebangInterpreters["python"] = saved })
	r := NewRuntimeAvailability("/data/plugins")
	zipPath := writeLauncherZip(t, "main.py")

	shebangInterpreters["python"] = []string{"sh"}
	require.NoError(t, r.CheckEntrypoint(zipPath, "python", "main.py"))

	shebangInterpreters["python"] = []string{"evey-no-such-python"}
	err := r.CheckEntrypoint(zipPath, "python", "main.py")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evey-no-such-python")
}

// An absolute path inside the plugin directories is a file of the plugin, so
// it cannot pass as a launcher of the image.
func TestCheckEntrypoint_AbsolutePluginPathIsNotALauncher(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "evil")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test plugin binary
	r := NewRuntimeAvailability(dir)
	require.Error(t, r.CheckEntrypoint(writeLauncherZip(t), "", bin))
	require.NoError(t, NewRuntimeAvailability("/data/plugins").CheckEntrypoint(writeLauncherZip(t), "", "/bin/sh"))
}

func TestRuntimeAvailability_AvailableIsSorted(t *testing.T) {
	got := NewRuntimeAvailability().Available()
	assert.True(t, slices.IsSorted(got), "%v", got)
	assert.Contains(t, got, "sh")
}
