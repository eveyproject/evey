package mkvgo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The retimer builds one shift entry per LATE audio track (keyed on the
// Matroska track number that mkvgo's AudioStartDelays reports natively), each
// the negation of that track's own delay, and leaves aligned tracks alone -
// the multi-track case the uniform ffmpeg remux refused. Mockable, no ffmpeg.
func TestAudioRetimer_PerTrackShiftMap(t *testing.T) {
	var gotShift map[uint64]int64
	r := &AudioRetimer{
		delays: func(_ context.Context, _ string) (map[uint64]int64, error) {
			return map[uint64]int64{2: 900_000_000, 3: 20_000_000}, nil
		},
		retime: func(_ context.Context, _ string, shift map[uint64]int64, _ ...extmatroska.Options) error {
			gotShift = shift
			return nil
		},
	}
	bak, skipped, err := r.RemuxAudioDelay(context.Background(), "movie.mkv")
	require.NoError(t, err)
	require.False(t, skipped)
	require.Len(t, gotShift, 1, "only the late track is shifted: %v", gotShift)
	assert.Equal(t, int64(-900_000_000), gotShift[2], "shifted back by its own delay, negated")
	assert.Empty(t, bak, "the mock produced no .bak, so no backup is reported")
}

func TestAudioRetimer_AlignedSkips(t *testing.T) {
	called := false
	r := &AudioRetimer{
		delays: func(_ context.Context, _ string) (map[uint64]int64, error) {
			return map[uint64]int64{2: 10_000_000, 3: 0}, nil
		},
		retime: func(_ context.Context, _ string, _ map[uint64]int64, _ ...extmatroska.Options) error {
			called = true
			return nil
		},
	}
	_, skipped, err := r.RemuxAudioDelay(context.Background(), "movie.mkv")
	require.NoError(t, err)
	assert.True(t, skipped)
	assert.False(t, called, "no retime when nothing is above the floor")
}

type fakeFallback struct{ called []string }

func (f *fakeFallback) RemuxAudioDelay(_ context.Context, p string) (string, bool, error) {
	f.called = append(f.called, p)
	return p + ".mkvgobak", false, nil
}

func TestAudioRetimer_MP4RoutesToFallback(t *testing.T) {
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "movie.mp4")
	require.NoError(t, os.WriteFile(mp4, []byte("not really mp4"), 0o644))
	fb := &fakeFallback{}
	r := NewAudioRetimer(fb)
	bak, skipped, err := r.RemuxAudioDelay(context.Background(), mp4)
	require.NoError(t, err)
	require.False(t, skipped)
	assert.Equal(t, []string{mp4}, fb.called)
	assert.Equal(t, mp4+".mkvgobak", bak)
}

func TestAudioRetimer_DelaysErrorRoutesToFallback(t *testing.T) {
	fb := &fakeFallback{}
	r := &AudioRetimer{
		delays:   func(_ context.Context, _ string) (map[uint64]int64, error) { return nil, assertErr("boom") },
		fallback: fb,
	}
	_, _, err := r.RemuxAudioDelay(context.Background(), "movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, []string{"movie.mkv"}, fb.called)
}

func TestAudioRetimer_MP4NoFallbackErrors(t *testing.T) {
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "movie.mp4")
	require.NoError(t, os.WriteFile(mp4, []byte("x"), 0o644))
	r := NewAudioRetimer(nil)
	_, _, err := r.RemuxAudioDelay(context.Background(), mp4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a Matroska")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
