package mkvgo

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pre-bump contract of the repair modes: ModeInPlace must FAIL EXPLICITLY
// (the operator refused a rewrite and the surgical strategy is stubbed);
// ModeRewrite and ModeAuto take the rewrite path (proven here by reaching its
// "open source" stage on a file that is EBML-labeled but not parseable —
// distinct from the in-place refusal error).
func TestReindexModes_PreBump(t *testing.T) {
	a := New()
	assert.False(t, a.SupportsInPlace(), "pre-bump build must not advertise in-place")

	if _, lookErr := exec.LookPath("ffmpeg"); lookErr != nil {
		t.Skip("ffmpeg not in PATH (run in the evey-server container)")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	// A REAL cue-less MKV: piping the muxer output makes it non-seekable, so
	// ffmpeg cannot write a Cues element — exactly the file class the repair
	// exists for (and the skip check must not trigger on it).
	gen := exec.Command("sh", "-c",
		"ffmpeg -hide_banner -v error -f lavfi -i testsrc2=size=64x64:rate=10 -t 1 -c:v libx264 -preset ultrafast -f matroska - > "+p)
	out, genErr := gen.CombinedOutput()
	require.NoError(t, genErr, string(out))

	_, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeInPlace})
	require.Error(t, err)
	assert.True(t, errors.Is(err, mkv.ErrInPlaceUnsupported),
		"ModeInPlace pre-bump must surface ErrInPlaceUnsupported, got: %v", err)

	// ModeRewrite repairs the file for real (backup kept), reporting its method.
	res, errRewrite := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeRewrite, KeepBackup: true})
	require.NoError(t, errRewrite)
	assert.Equal(t, mkv.ReindexMethodRewrite, res.Method)
	assert.FileExists(t, p+".mkvgobak")
	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	assert.True(t, has, "the rewritten file must now carry a real Cues index")

	// ModeAuto pre-bump falls back to the rewrite path: restore the original
	// (cue-less) first, then repair again without naming a mode.
	require.NoError(t, a.RestoreBackup(context.Background(), p))
	resAuto, errAuto := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{KeepBackup: false})
	require.NoError(t, errAuto)
	assert.Equal(t, mkv.ReindexMethodRewrite, resAuto.Method)
	assert.NoFileExists(t, p+".mkvgobak", "KeepBackup=false removes the backup after the swap")
}
