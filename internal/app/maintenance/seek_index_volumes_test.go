package maintenance_test

import (
	"os"
	"path/filepath"
	"testing"

	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Evey must advise for or against backups ITSELF: the scan already knows every
// repair candidate and the volumes they live on, so the response carries the
// per-filesystem numbers (free space, sum and max of the un-indexed sizes)
// the UI needs to pre-set the safe choice instead of letting the operator
// discover a full disk mid-batch.
func TestVolumeSummary(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, size int) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, make([]byte, size), 0o644))
		return p
	}
	items := []appmaintenance.SeekStatus{
		{Path: mk("a.mkv", 100), HasIndex: false, SizeBytes: 100},
		{Path: mk("b.mkv", 300), HasIndex: false, SizeBytes: 300},
		// Indexed: healthy, must not count toward the repair estimate.
		{Path: mk("c.mkv", 1000), HasIndex: true, SizeBytes: 1000},
		// Vanished between scan and summary: skipped, never fatal.
		{Path: filepath.Join(dir, "gone.mkv"), HasIndex: false, SizeBytes: 999},
	}

	vols := appmaintenance.VolumeSummary(items)
	require.Len(t, vols, 1, "all files sit on one filesystem")
	v := vols[0]
	assert.Equal(t, int64(400), v.MissingBytes)
	assert.Equal(t, int64(300), v.LargestMissingBytes)
	assert.Equal(t, 2, v.MissingFiles)
	assert.Positive(t, v.FreeBytes, "statfs must report the live free space")
	assert.Equal(t, dir, v.Root, "the shortest observed directory names the volume")
}

func TestVolumeSummary_NoCandidates(t *testing.T) {
	assert.Empty(t, appmaintenance.VolumeSummary([]appmaintenance.SeekStatus{
		{Path: "/nonexistent/x.mkv", HasIndex: true},
	}))
	assert.Empty(t, appmaintenance.VolumeSummary(nil))
}
