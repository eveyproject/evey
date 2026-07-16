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

type refusingFallback struct{ err error }

func (f *refusingFallback) RemuxAudioDelay(_ context.Context, _ string) (string, bool, error) {
	return "", false, f.err
}

// THE PROD BLIND SPOT (2026-07-14): 64 files logged only the ffmpeg fallback's
// "tracks disagree on the delay" refusal - mkvgo's own reason for handing the
// file over had been swallowed, so the logs could not say whether the primary
// engine ever ran, or why it refused. When BOTH engines refuse, the error must
// carry BOTH refusals.
func TestAudioRetimer_DoubleRefusalCarriesBothReasons(t *testing.T) {
	fb := &refusingFallback{err: assertErr("audio-remux: the 2 audio tracks disagree on the delay")}
	r := &AudioRetimer{
		delays: func(_ context.Context, _ string) (map[uint64]int64, error) {
			return map[uint64]int64{2: 900_000_000}, nil
		},
		retime: func(_ context.Context, _ string, _ map[uint64]int64, _ ...extmatroska.Options) error {
			return assertErr("retime: unknown-size segment")
		},
		fallback: fb,
	}
	_, _, err := r.RemuxAudioDelay(context.Background(), "movie.mkv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown-size segment", "mkvgo's own refusal must survive the fallback")
	assert.Contains(t, err.Error(), "disagree on the delay", "the fallback's refusal stays too")
}

// A fallback that SUCCEEDS still repairs the file: mkvgo's refusal is then
// irrelevant and must not surface as an error.
func TestAudioRetimer_RetimeRefusalFallbackSuccessIsClean(t *testing.T) {
	fb := &fakeFallback{}
	r := &AudioRetimer{
		delays: func(_ context.Context, _ string) (map[uint64]int64, error) {
			return map[uint64]int64{2: 900_000_000}, nil
		},
		retime: func(_ context.Context, _ string, _ map[uint64]int64, _ ...extmatroska.Options) error {
			return assertErr("retime: unknown-size segment")
		},
		fallback: fb,
	}
	bak, skipped, err := r.RemuxAudioDelay(context.Background(), "movie.mkv")
	require.NoError(t, err)
	require.False(t, skipped)
	assert.Equal(t, "movie.mkv.mkvgobak", bak)
}

// Same for the delays-measurement path: when measuring fails AND the fallback
// fails, neither reason may vanish.
func TestAudioRetimer_DelaysErrorThenFallbackErrorCarriesBoth(t *testing.T) {
	fb := &refusingFallback{err: assertErr("ffprobe exploded")}
	r := &AudioRetimer{
		delays:   func(_ context.Context, _ string) (map[uint64]int64, error) { return nil, assertErr("ebml header: EOF") },
		fallback: fb,
	}
	_, _, err := r.RemuxAudioDelay(context.Background(), "movie.mkv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ebml header: EOF")
	assert.Contains(t, err.Error(), "ffprobe exploded")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
