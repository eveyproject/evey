package prober

import (
	"context"
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
