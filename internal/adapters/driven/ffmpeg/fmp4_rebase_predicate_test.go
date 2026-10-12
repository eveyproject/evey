package ffmpeg

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// A transcode seek must be rebased whichever of the two seek fields the caller
// filled: the orchestrator only ever set StartSec, and keying the predicate on
// StartNumber alone let every seek into a transcoded title publish 0-based
// fragments into the shared cache (prod, 2026-09-02/03: "stays in buffering").
func TestNeedsFMP4Rebase_TranscodeSeekByEitherField(t *testing.T) {
	cases := []struct {
		name string
		o    transcoding.HLSProcessOpts
		want bool
	}{
		{"cold transcode", transcoding.HLSProcessOpts{UseFMP4: true, VideoDecision: "transcode"}, false},
		{"transcode seek by StartNumber", transcoding.HLSProcessOpts{UseFMP4: true, VideoDecision: "transcode", StartNumber: 129}, true},
		{"transcode seek by StartSec only", transcoding.HLSProcessOpts{UseFMP4: true, VideoDecision: "transcode", StartSec: 387}, true},
		{"copy always", transcoding.HLSProcessOpts{UseFMP4: true, VideoDecision: "copy"}, true},
		{"mpeg-ts never", transcoding.HLSProcessOpts{UseFMP4: false, VideoDecision: "transcode", StartSec: 387, StartNumber: 129}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsFMP4Rebase(tc.o); got != tc.want {
				t.Fatalf("needsFMP4Rebase(%+v) = %v, want %v", tc.o, got, tc.want)
			}
		})
	}
}
