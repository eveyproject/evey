package pluginrunner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), make([]byte, 1000), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b"), make([]byte, 2000), 0o644))

	sz, err := dirSize(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), sz, "must sum regular files recursively")
}

func TestDirSize_Empty(t *testing.T) {
	sz, err := dirSize(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(0), sz)
}

func TestWatchDiskQuota_DisabledReturnsImmediately(t *testing.T) {
	r := NewRunner() // diskMaxBytes == 0 → disabled
	// Must return at once (no ticker, no goroutine that outlives the plugin);
	// if the guard were missing this would block on the 30s ticker.
	r.watchDiskQuota(&processEntry{done: make(chan struct{})},
		&plugin.Plugin{ID: "x", InstallDir: t.TempDir()})
}
