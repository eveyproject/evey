package ffmpeg

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// startNumberFor must derive the -start_number from StartSec for transcode
// (uniform segments) but take the absolute index verbatim for copy
// (keyframe-aligned variable-length segments) — otherwise a forward seek emits
// the wrong stream_N_<idx>.m4s and the player loads forever.
func TestStartNumberFor(t *testing.T) {
	cases := []struct {
		name string
		opts transcoding.HLSProcessOpts
		want int
	}{
		{"transcode cold start", transcoding.HLSProcessOpts{VideoDecision: "transcode", StartSec: 0}, 0},
		{"transcode mid-stream derives floor(StartSec/3)", transcoding.HLSProcessOpts{VideoDecision: "transcode", StartSec: 153}, 51},
		{"copy cold start", transcoding.HLSProcessOpts{VideoDecision: "copy", StartNumber: 0, StartSec: 0}, 0},
		{
			// The seek bug: a copy seek to plan-segment 80 lands at a keyframe time
			// (520s) whose floor(520/3)=173 ≠ 80. start_number must be the absolute
			// plan index (80), not the time-derived value.
			"copy seek uses absolute index, not StartSec/segLen",
			transcoding.HLSProcessOpts{VideoDecision: "copy", StartNumber: 80, StartSec: 520},
			80,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startNumberFor(tc.opts); got != tc.want {
				t.Fatalf("startNumberFor(%+v) = %d, want %d", tc.opts, got, tc.want)
			}
		})
	}
}
