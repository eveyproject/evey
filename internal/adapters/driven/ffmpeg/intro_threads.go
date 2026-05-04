package ffmpeg

import (
	"os"
	"strconv"
	"sync"
)

// introFFmpegThreadsValue is the cached `-threads N` flag value used by
// every ffmpeg invocation in the intro detection adapters
// (Chromaprinter.Compute, IntroDetector.runDetect, AudioFallback,
// DetectEndCredits). Bounded by EVEY_INTRO_FFMPEG_THREADS, default 1.
//
// 1 keeps the per-process budget predictable: with `MaxParallelDetections`
// items in flight at once and one ffmpeg per item-step (the chromaprint
// loop is sequential across siblings), the host's total ffmpeg thread
// usage is bounded by `MaxParallelDetections × N`. Audio-only decodes
// (Chromaprint) gain nothing from multithreading; silencedetect /
// blackdetect over a +/-search window are fast enough at threads=1 for
// the 4-minute intro region they target.
//
// Power users with idle CPU budget can raise this via the env var.
var (
	introFFmpegThreadsOnce  sync.Once
	introFFmpegThreadsValue int
)

func introFFmpegThreads() int {
	introFFmpegThreadsOnce.Do(func() {
		introFFmpegThreadsValue = 1
		if v := os.Getenv("EVEY_INTRO_FFMPEG_THREADS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 16 {
				introFFmpegThreadsValue = n
			}
		}
	})
	return introFFmpegThreadsValue
}
