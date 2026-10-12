package mkvgo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file with a corrupt region between clusters is REFUSED by the strict
// reindex but repaired by the Resync path: it skips the damaged span,
// resynchronises, keeps a .mkvgobak backup, and reports the lost range. This
// is the exact prod defect class ("invalid VINT" / "unexpected EOF" mid-file).
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestReindexResync -v
func TestReindexResyncIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	ctx := t.Context()
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.mkv")
	// A multi-cluster no-Cue MKV: ffmpeg writes to its OWN stdout ("-f matroska
	// -"), which the shell redirects to clean - a real pipe, so ffmpeg cannot
	// seek back to write Cues (unlike passing `clean` as ffmpeg's own output
	// argument, which hands it a seekable file and gets a real Cues element
	// mkvgo can always rediscover, defeating this fixture's whole premise).
	gen := exec.CommandContext(ctx, "sh", "-c",
		"ffmpeg -hide_banner -v error -y -f lavfi -i testsrc2=size=320x180:rate=24 "+
			"-f lavfi -i sine=frequency=300 -t 40 -c:v libx264 -preset ultrafast -g 48 "+
			"-c:a aac -cluster_time_limit 500 -f matroska - > "+clean)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}

	// Splice 4 KiB of junk into the middle (between clusters) to create the
	// corrupt region the strict reindex refuses.
	corrupt := filepath.Join(dir, "corrupt.mkv")
	data, err := os.ReadFile(clean)
	require.NoError(t, err)
	mid := len(data) / 2
	junk := make([]byte, 4096)
	for i := range junk {
		junk[i] = byte(i * 7)
	}
	spliced := append(append(append([]byte{}, data[:mid]...), junk...), data[mid:]...)
	require.NoError(t, os.WriteFile(corrupt, spliced, 0o644))

	a := New()

	// Strict reindex refuses the corrupt file (leaves it intact).
	_, strictErr := a.ReindexInPlace(ctx, corrupt, mkv.ReindexOptions{Mode: mkv.ModeRewrite})
	require.Error(t, strictErr, "strict reindex must refuse a corrupted region")

	// Resync repairs it.
	res, err := a.ReindexInPlace(ctx, corrupt, mkv.ReindexOptions{Resync: true})
	require.NoError(t, err, "resync repairs the corrupt file")
	assert.False(t, res.Skipped)
	assert.Equal(t, corrupt+".mkvgobak", res.BackupPath, "a backup is forced (data is lost)")
	_, bakErr := os.Stat(res.BackupPath)
	require.NoError(t, bakErr, "the .mkvgobak backup exists")
	require.NotEmpty(t, res.DamagedRanges, "the skipped span is reported")
	// The repaired file now has a real seek index.
	has, err := a.HasSeekIndex(ctx, corrupt)
	require.NoError(t, err)
	assert.True(t, has, "the repaired file carries a Cues index")
}

// A real damaged file (CELLAR test7: an element running past the end of the
// file mid-stream) repaired the way the admin's Repair button does: the output
// decodes without a single ffmpeg error, because video resumes at a keyframe
// instead of keeping the frames whose references the damage took.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestReindexResyncRealDamage -v
func TestReindexResyncRealDamageDecodesClean(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx := t.Context()
	src := suiteFile(t, "test7.mkv")
	path := filepath.Join(t.TempDir(), "test7.mkv")
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	res, err := New().ReindexInPlace(ctx, path, mkv.ReindexOptions{Resync: true})
	require.NoError(t, err)
	require.NotEmpty(t, res.DamagedRanges, "the damaged span is reported")

	out, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, string(out), "the repaired file must decode without an error")
}
