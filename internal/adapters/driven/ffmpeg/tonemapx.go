package ffmpeg

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/pkg/procfork"
)

var (
	tonemapxOnce sync.Once
	tonemapxOK   bool
)

// tonemapxAvailable reports whether the active ffmpeg build provides
// jellyfin-ffmpeg's `tonemapx` filter - an AVX2/SIMD HDR->SDR tonemap measured
// ~1.8x faster than the portable zscale float32 chain on the same CPU (old box,
// 1080p: 1.56x vs 0.89x realtime). Probed once and cached: vanilla ffmpeg lacks
// the filter, so this returns false and BuildToneMapFilter falls back to the
// zscale chain. Any probe error (ffmpeg missing, timeout) is also false - the
// fallback is always safe, the fast path is opt-in on its presence.
func tonemapxAvailable(ffmpegPath string) bool {
	tonemapxOnce.Do(func() {
		if ffmpegPath == "" {
			ffmpegPath = "ffmpeg"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "tonemapx.filters"}, ffmpegPath, "-hide_banner", "-filters")
		tonemapxOK = err == nil && bytes.Contains(out, []byte("tonemapx"))
	})
	return tonemapxOK
}
