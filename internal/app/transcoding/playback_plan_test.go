package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The cost axis is the whole point of PlaybackPlan: "who does the work, at what
// price" must be a queryable property, not something a reader reconstructs from
// the decision. Lock the three packagers' semantics.
func TestPlaybackPlan_CostHelpers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		packager    PackagerKind
		wantEncode  bool // spends encode CPU/GPU (billed by the limiter)
		wantsFFmpeg bool // spawns any ffmpeg process (not the zero-process mkvgo path)
	}{
		{"mkvgo copy = free", PackagerCopyMkvgo, false, false},
		{"ffmpeg copy = process, no encode", PackagerCopyFFmpeg, false, true},
		{"ffmpeg transcode = costly", PackagerTranscodeFFmpeg, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := PlaybackPlan{Packager: tc.packager}
			assert.Equal(t, tc.wantEncode, p.NeedsVideoEncode())
			assert.Equal(t, tc.wantsFFmpeg, p.UsesFFmpeg())
		})
	}
}
