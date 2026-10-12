//go:build trickplaylive

package ffmpeg

// trickplay_live_test.go - both sprite paths on real files, with the real
// ffmpeg and mkvgo: go test -tags trickplaylive -run TestTrickplayLive with
// TRICKPLAY_LIVE_FILES (colon-separated paths) and TRICKPLAY_LIVE_OUT.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gravity-zero/mkvgo/matroska"
	"github.com/stretchr/testify/require"

	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
)

type liveKeyframes struct {
	a    *mkvgoadapter.Adapter
	last *error // the last stream's outcome, to tell which path Generate took
}

func (k liveKeyframes) WriteKeyframeStream(ctx context.Context, p string, everyMs int64, w io.Writer) (KeyframeStreamStats, error) {
	s, err := k.a.WriteKeyframeStream(ctx, p, everyMs, w)
	*k.last = err
	// Same translation as cmd/evey's trickplayKeyframes.
	if errors.Is(err, matroska.ErrNoKeyframeIndex) || errors.Is(err, matroska.ErrNoVideo) || errors.Is(err, matroska.ErrUnsupportedCodec) {
		err = fmt.Errorf("%w: %w", ErrKeyframesUnavailable, err)
	}
	return KeyframeStreamStats{Samples: s.Samples, PtsMs: s.PtsMs, Skipped: s.Skipped,
		BytesRead: s.BytesRead + s.MetaBytesRead, MedianCueGapMs: s.MedianCueGapMs}, err
}

func (k liveKeyframes) CuesMissKeyframes(ctx context.Context, p string) (bool, error) {
	return k.a.CuesMissKeyframes(ctx, p)
}

func TestTrickplayLive(t *testing.T) {
	files := strings.Split(os.Getenv("TRICKPLAY_LIVE_FILES"), ":")
	out := os.Getenv("TRICKPLAY_LIVE_OUT")
	if files[0] == "" || out == "" {
		t.Skip("TRICKPLAY_LIVE_FILES / TRICKPLAY_LIVE_OUT not set")
	}
	const interval, width, tw, th = 7, 320, 5, 5
	var last error
	g := NewTrickplayGenerator("ffmpeg", "ffprobe").WithKeyframeStreamer(liveKeyframes{mkvgoadapter.New(), &last})
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tail := spriteFilterTail(width, tw, th, g.probeColour(ctx, f))
			last = errors.New("keyframe stream not attempted")
			t0 := time.Now()
			kf, err := g.Generate(ctx, f, filepath.Join(out, name, "generate"), interval, width, tw, th, 1, 0)
			kfWall := time.Since(t0)
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Join(out, name, "scan"), 0o755))
			t0 = time.Now()
			full, err := g.generateFullScan(ctx, f, filepath.Join(out, name, "scan"), interval, tail, 1)
			fullWall := time.Since(t0)
			require.NoError(t, err)
			t.Logf("RESULT %s path_err=%v | generate: %d thumbs, %d sprites, %s | scan: %d thumbs, %d sprites, %s",
				name, last, len(kf.FrameTimestampsMs), kf.ImageCount, kfWall.Round(time.Millisecond),
				len(full.FrameTimestampsMs), full.ImageCount, fullWall.Round(time.Millisecond))
			if strings.Contains(os.Getenv("TRICKPLAY_LIVE_SCAN_DIFFERS"), name) {
				// Sources the scan samples differently: a decoder that ignores
				// -skip_frame (VP9, inter frames picked), open-GOP H.264 (IDRs
				// only), CRA-keyed HEVC (stale POCs). Order and coverage compare;
				// the pictures are checked against a full decode separately.
				for i := 1; i < len(kf.FrameTimestampsMs); i++ {
					require.Greater(t, kf.FrameTimestampsMs[i], kf.FrameTimestampsMs[i-1])
				}
				require.GreaterOrEqual(t, len(kf.FrameTimestampsMs), len(full.FrameTimestampsMs)*3/4)
				return
			}
			// Same sampling rule on the same keyframes: the strip the scan makes.
			require.Equal(t, full.FrameTimestampsMs, kf.FrameTimestampsMs)
			require.Equal(t, full.ImageCount, kf.ImageCount)
		})
	}
}
