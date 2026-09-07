package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// A decoupled session encodes on the GPU but decodes on the CPU, a mix no
// calibration has ever benched. Its ladder must therefore be capped at the
// SOFTWARE ceiling, not the hardware one: advertising a rung the pipeline
// cannot sustain is what starves a player (ADR-0017).
func TestOrchestratorProfiles_DecoupledSourceCapsAtSoftwareCeiling(t *testing.T) {
	cache, _ := newTestCache(t)
	orch := NewSegmentOrchestrator(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, testOpts(), discardLogger())
	defer orch.Close()

	capability := transcoding.EncodeCapability{
		SoftwareMaxHeight: 720,
		HWMaxHeight:       2160,
		HWBackend:         "vaapi",
		HWCodecs:          []string{"h264"},
		HWDecodeCodecs:    []string{"h264", "hevc"},
	}
	base := transcoding.LadderFilmNormal
	src := SourceDims{Width: 1920, Height: 1080}

	t.Run("HW-decodable source keeps the hardware ceiling", func(t *testing.T) {
		s := src
		s.Codec = "h264"
		profiles := orch.Profiles(base, s, transcoding.PlaybackDecision{}, capability)
		assert.Equal(t, 1080, profiles[len(profiles)-1].Height,
			"a zero-copy session is capped by the source, not by the software ceiling")
	})

	t.Run("undecodable source is capped by the software ceiling", func(t *testing.T) {
		s := src
		s.Codec = "mpeg4" // DivX/XviD: no Intel iGPU decodes MPEG-4 ASP
		profiles := orch.Profiles(base, s, transcoding.PlaybackDecision{}, capability)
		for _, p := range profiles {
			assert.LessOrEqual(t, p.Height, 720,
				"the CPU decodes this source: no rung may exceed the software ceiling")
		}
	})

	t.Run("unknown source codec keeps the hardware ceiling", func(t *testing.T) {
		profiles := orch.Profiles(base, src, transcoding.PlaybackDecision{}, capability)
		assert.Equal(t, 1080, profiles[len(profiles)-1].Height,
			"an unidentified source skips the decode gate, exactly as ResolveHW does")
	})
}
