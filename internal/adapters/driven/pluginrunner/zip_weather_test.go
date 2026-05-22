package pluginrunner

import (
	"os"
	"path/filepath"
	"testing"
)

// Verifies the real installer Extract path on the regenerated weather.zip:
// main.js must land at the install-dir root and the backend's @grpc runtime
// dep must be present, otherwise the runner reports "script not found" or the
// node process crashes with "Cannot find module".
func TestExtract_WeatherZip_PlacesMainAndDeps(t *testing.T) {
	zipPath := "../../../../tests/plugins/weather.zip"
	if _, err := os.Stat(zipPath); err != nil {
		t.Skipf("weather.zip not present: %v", err)
	}
	dest := t.TempDir()
	if err := NewZipInstaller().Extract(zipPath, dest); err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	for _, rel := range []string{
		"main.js",
		"plugin.yaml",
		"node_modules/@grpc/grpc-js",
		"node_modules/@grpc/proto-loader",
	} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Errorf("expected %q in install dir, got: %v", rel, err)
		}
	}
}
