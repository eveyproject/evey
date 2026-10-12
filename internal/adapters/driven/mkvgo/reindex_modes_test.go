package mkvgo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	extwriter "github.com/gravity-zero/mkvgo/mkv/writer"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genCuelessMKV writes a REAL cue-less MKV: piping the muxer output makes it
// non-seekable, so ffmpeg cannot write a Cues element - exactly the file
// class the repair exists for (and the skip check must not trigger on it).
func genCuelessMKV(t *testing.T) string {
	t.Helper()
	if _, lookErr := exec.LookPath("ffmpeg"); lookErr != nil {
		t.Skip("ffmpeg not in PATH (run in the evey-server container)")
	}
	p := filepath.Join(t.TempDir(), "f.mkv")
	gen := exec.Command("sh", "-c",
		"ffmpeg -hide_banner -v error -f lavfi -i testsrc2=size=64x64:rate=10 -t 1 -c:v libx264 -preset ultrafast -f matroska - > "+p)
	out, genErr := gen.CombinedOutput()
	require.NoError(t, genErr, string(out))
	return p
}

// genCuelessKnownSizeMKV writes a REAL cue-less MKV with a sealed (known)
// Segment size, built directly with mkvgo's writer - unlike genCuelessMKV's
// piped ffmpeg output, this one is genuinely in-place-eligible (v0.21.1:
// ReindexInPlace correctly refuses an unknown-size Segment, since its
// crash-safety window depends on writing past a declared end that doesn't
// exist yet). WriteClusterWithCues accumulates into m.Cues as it writes the
// real cluster bytes; clearing it before Finalize keeps the clusters and the
// sealed size but drops the Cues element itself - a real encoder's cueless
// output, minus the "can't seek back" excuse.
func genCuelessKnownSizeMKV(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sealed.mkv")
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()

	mw := extwriter.NewMKVWriter(f)
	require.NoError(t, mw.WriteStart())
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: 1_000_000}}
	tracks := []extmkv.Track{{ID: 1, Type: extmkv.VideoTrack, Codec: "h264", Language: "eng", IsDefault: true}}
	require.NoError(t, mw.WriteMetadata(c, tracks, 3000))
	for _, ts := range []int64{0, 1000, 2000} {
		blocks := []extmkv.Block{{TrackNumber: 1, Timecode: ts, Keyframe: true, Data: []byte("frame")}}
		require.NoError(t, mw.WriteClusterWithCues(ts, 1_000_000, blocks))
	}
	mw.Cues = nil // drop the accumulated index: real clusters, no Cues element
	require.NoError(t, mw.Finalize())
	return p
}

// Post-bump (mkvgo v0.17.0) contract of the repair modes: the surgical
// in-place strategy is live. ModeInPlace patches the file directly (no temp,
// no backup); ModeAuto prefers it; ModeRewrite still takes the temp+swap
// path with Evey's backup semantics.
func TestReindexModes_InPlaceLive(t *testing.T) {
	a := New()
	assert.True(t, a.SupportsInPlace(), "post-bump build must advertise in-place")

	p := genCuelessKnownSizeMKV(t)

	res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeInPlace})
	require.NoError(t, err, "a known-size, SeekHead-less layout patches in place")
	assert.Equal(t, mkv.ReindexMethodInPlace, res.Method)
	assert.Empty(t, res.BackupPath, "in-place never duplicates the file")
	assert.NoFileExists(t, p+".mkvgobak")

	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	assert.True(t, has, "the patched file must now carry a real, parseable Cues index")

	// Idempotence: a file that now has an index is skipped, not re-patched.
	res2, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeInPlace})
	require.NoError(t, err)
	assert.True(t, res2.Skipped)
}

func TestReindexModes_RewriteKeepsBackupSemantics(t *testing.T) {
	a := New()
	p := genCuelessMKV(t)

	// The operator explicitly refused the patch: full rewrite, backup kept.
	res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeRewrite, KeepBackup: true})
	require.NoError(t, err)
	assert.Equal(t, mkv.ReindexMethodRewrite, res.Method)
	assert.FileExists(t, p+".mkvgobak")
	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	assert.True(t, has, "the rewritten file must now carry a real Cues index")
}

// A media file the server user cannot write must surface an ACTIONABLE
// permission error - not masquerade as "in-place reindex not supported for
// this file or build" (prod incident 2026-07-10: 261 identical cryptic
// failures on a PUID/media-owner mismatch).
func TestReindexModes_PermissionDeniedIsActionable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission bits do not apply to root")
	}
	a := New()

	t.Run("inplace mode names the real cause", func(t *testing.T) {
		p := genCuelessMKV(t)
		require.NoError(t, os.Chmod(p, 0o444))
		_, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeInPlace})
		require.Error(t, err)
		require.ErrorIs(t, err, mkv.ErrMediaNotWritable)
		require.NotErrorIs(t, err, mkv.ErrInPlaceUnsupported,
			"EACCES is an ownership problem, not a layout/build refusal")
		assert.Contains(t, err.Error(), "PUID", "the error must tell the operator the remedy")
	})

	t.Run("auto mode still rescues via the swap rewrite when the directory allows it", func(t *testing.T) {
		p := genCuelessMKV(t)
		require.NoError(t, os.Chmod(p, 0o444))
		res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{})
		require.NoError(t, err, "the temp+rename rewrite only needs directory write, not file write")
		assert.Equal(t, mkv.ReindexMethodRewrite, res.Method)
	})

	t.Run("auto mode names the real cause when nothing is writable", func(t *testing.T) {
		p := genCuelessMKV(t)
		dir := filepath.Dir(p)
		require.NoError(t, os.Chmod(p, 0o444))
		require.NoError(t, os.Chmod(dir, 0o555))
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		_, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{})
		require.Error(t, err)
		require.ErrorIs(t, err, mkv.ErrMediaNotWritable)
		assert.Contains(t, err.Error(), "PUID")
	})
}

func TestReindexModes_AutoPrefersInPlace(t *testing.T) {
	a := New()
	p := genCuelessKnownSizeMKV(t)

	res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{})
	require.NoError(t, err)
	assert.Equal(t, mkv.ReindexMethodInPlace, res.Method, "ModeAuto must prefer the surgical patch when it applies")
	assert.NoFileExists(t, p+".mkvgobak", "no backup on the in-place path")
	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	assert.True(t, has)
}
