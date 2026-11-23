package prober

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKeyframeProber struct {
	ret    []int64
	called int
}

func (f *fakeKeyframeProber) ProbeKeyframes(context.Context, string) ([]int64, error) {
	f.called++
	return f.ret, nil
}

// When mkvgo yields no seek index - gated off, unsupported container, or a file
// it cannot open - the dispatcher must defer to the ffprobe packet scan and
// return its result unchanged. (The fast-path success is covered by the
// real-file keyframe parity, which is PATH-gated.)
func TestKeyframeDispatcher_FallsBackWhenNoIndex(t *testing.T) {
	want := []int64{0, 5000, 10000}
	ctx := context.Background()

	cases := []struct {
		name         string
		mkvOn, mp4On bool
		path         string
	}{
		{"both gates off, mp4 path", false, false, "/nope/movie.mp4"},
		{"both gates off, mkv path", false, false, "/nope/movie.mkv"},
		{"gates open but unsupported container", true, true, "/nope/movie.avi"},
		{"gates open, mp4 path but unreadable file", true, true, "/nope/missing.mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fb := &fakeKeyframeProber{ret: want}
			d := NewKeyframeDispatcher(fb, func() bool { return tc.mkvOn }, func() bool { return tc.mp4On })
			got, err := d.ProbeKeyframes(ctx, tc.path)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.Equal(t, 1, fb.called, "ffprobe fallback must be invoked exactly once")
		})
	}
}

// The seek-index-only read answers from the Cues a Matroska carries and never
// walks one without: that walk reads the whole file, which is what must not
// run beside a live stream. The full dispatcher still walks it.
func TestKeyframeDispatcher_SeekIndexOnlyNeverWalks(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	dir := t.TempDir()
	cued := filepath.Join(dir, "cued.mkv")
	gen := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-t", "6",
		"-c:v", "libx264", "-g", "20", "-pix_fmt", "yuv420p", "-f", "matroska"}
	require.NoError(t, exec.Command(ffmpeg, append(gen, cued)...).Run())
	// Written to a pipe, the muxer cannot seek back: no Cues.
	var cueless bytes.Buffer
	cmd := exec.Command(ffmpeg, append(gen, "pipe:1")...)
	cmd.Stdout = &cueless
	require.NoError(t, cmd.Run())
	bare := filepath.Join(dir, "bare.mkv")
	require.NoError(t, os.WriteFile(bare, cueless.Bytes(), 0o600))

	ctx := context.Background()
	fb := &fakeKeyframeProber{}
	d := NewKeyframeDispatcher(fb, nil, nil)
	seek := d.SeekIndexOnly()

	kf, err := seek.ProbeKeyframes(ctx, cued)
	require.NoError(t, err)
	assert.NotEmpty(t, kf)

	kf, err = seek.ProbeKeyframes(ctx, bare)
	require.Error(t, err)
	assert.Empty(t, kf)
	assert.Zero(t, fb.called, "no ffprobe fallback either")

	kf, err = d.ProbeKeyframes(ctx, bare)
	require.NoError(t, err)
	assert.Len(t, kf, 3, "the full dispatcher walks it: one keyframe every 2 s over 6 s")
}
