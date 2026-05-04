package ffmpeg

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// resetIntroThreadsCache forces the next call to introFFmpegThreads to
// re-read the environment. The production code memoises with sync.Once
// so we have to swap the once + value to test different env values
// inside the same process.
func resetIntroThreadsCache() {
	introFFmpegThreadsOnce = sync.Once{}
	introFFmpegThreadsValue = 0
}

// Default: 1 thread per ffmpeg invocation. Pinned because the whole
// budget calculation downstream assumes "at most concurrencyLimiter
// items × N threads".
func TestIntroFFmpegThreads_DefaultIsOne(t *testing.T) {
	resetIntroThreadsCache()
	t.Setenv("EVEY_INTRO_FFMPEG_THREADS", "")
	assert.Equal(t, 1, introFFmpegThreads())
}

// Override accepts a sane positive value.
func TestIntroFFmpegThreads_EnvOverride(t *testing.T) {
	resetIntroThreadsCache()
	t.Setenv("EVEY_INTRO_FFMPEG_THREADS", "4")
	assert.Equal(t, 4, introFFmpegThreads())
}

// Out-of-range / non-numeric values fall back to the default rather
// than crash or saturate the host.
func TestIntroFFmpegThreads_RejectsGarbage(t *testing.T) {
	cases := []string{"-1", "0", "999", "abc", "1.5"}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			resetIntroThreadsCache()
			t.Setenv("EVEY_INTRO_FFMPEG_THREADS", c)
			assert.Equal(t, 1, introFFmpegThreads(),
				"value %q must fall back to the default", c)
		})
	}
}
