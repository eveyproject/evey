package httpapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

func topHeight(ps []domaintranscoding.HLSProfile) int {
	m := 0
	for _, p := range ps {
		if p.Height > m {
			m = p.Height
		}
	}
	return m
}

// The read-verbatim payoff (Étape 2): a stream's ABR ladder is shaped ONCE and
// frozen on its admission pin, so a mid-stream drift of a mutable shaping input
// (here the CMCD adaptive override, but equally a complexity re-class or a
// capability demotion) can NOT change the ladder a live player already has - the
// manifest's URLs and every segment stay addressed to the same cached bytes BY
// CONSTRUCTION, not by hoping every re-derivation agrees.
func TestProfilesForDecision_FrozenLadder_ImmuneToMidStreamDrift(t *testing.T) {
	w, hgt := 3840, 2160
	streams := []*domainlibrary.MediaStream{{StreamType: "video", Codec: "h264", Width: &w, Height: &hgt}}
	dec := domaintranscoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	item := &domainlibrary.MediaItem{ID: "mi-drift"}

	shaper := apptranscoding.NewVideoAdmissionShaper(apptranscoding.NewVideoSessionLimiter(0))

	var overrideH int // the mutable mid-stream input the test flips
	h := &hlsVODServe{
		capability:            func() domaintranscoding.EncodeCapability { return domaintranscoding.EncodeCapability{SoftwareMaxHeight: 2160} },
		admissionRoute:        admissionRouteOf(shaper),
		admissionPinnedClass:  pinnedClassOf(shaper),
		admissionFrozenLadder: frozenLadderOf(shaper),
		admissionFreezeLadder: freezeLadderOf(shaper),
		capOverride: func(string, int) (domaintranscoding.CapOverride, bool) {
			if overrideH > 0 {
				return domaintranscoding.CapOverride{MaxHeight: overrideH}, true
			}
			return domaintranscoding.CapOverride{}, false
		},
	}
	ctx := context.Background()

	// Manifest time: the override caps the ladder to 1080p. Shaped + FROZEN.
	overrideH = 1080
	manifestTop := topHeight(h.profilesForDecision(ctx, item, streams, dec))
	require.Equal(t, 1080, manifestTop, "manifest ladder capped to the current override")

	// Mid-stream the override drops to 480p. A segment request for the SAME stream
	// must still shape the 1080p ladder - it reads the frozen plan, never the
	// drifted input.
	overrideH = 480
	segmentTop := topHeight(h.profilesForDecision(ctx, item, streams, dec))
	assert.Equal(t, manifestTop, segmentTop, "frozen ladder is read verbatim, immune to the mid-stream drift")

	// Control: a DIFFERENT stream (its own fresh pin) DOES see the drifted 480p,
	// proving the override really bites and the 1080p above was the freeze at work,
	// not a no-op.
	controlTop := topHeight(h.profilesForDecision(ctx, &domainlibrary.MediaItem{ID: "mi-other"}, streams, dec))
	assert.Less(t, controlTop, manifestTop, "a fresh stream sees the current (drifted) override - the freeze is per-stream")
	assert.Equal(t, 480, controlTop)
}

// Without an admission shaper wired (hand-built handlers, the pre-Étape-2 path),
// profilesForDecision must keep re-deriving every call - the freeze is purely
// additive and never changes behaviour when it is not wired.
func TestProfilesForDecision_NoShaper_StillReDerivesEachCall(t *testing.T) {
	w, hgt := 3840, 2160
	streams := []*domainlibrary.MediaStream{{StreamType: "video", Codec: "h264", Width: &w, Height: &hgt}}
	dec := domaintranscoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	item := &domainlibrary.MediaItem{ID: "mi-noshaper"}

	var overrideH int
	h := &hlsVODServe{
		capability: func() domaintranscoding.EncodeCapability { return domaintranscoding.EncodeCapability{SoftwareMaxHeight: 2160} },
		capOverride: func(string, int) (domaintranscoding.CapOverride, bool) {
			if overrideH > 0 {
				return domaintranscoding.CapOverride{MaxHeight: overrideH}, true
			}
			return domaintranscoding.CapOverride{}, false
		},
	}
	ctx := context.Background()

	overrideH = 1080
	first := topHeight(h.profilesForDecision(ctx, item, streams, dec))
	overrideH = 480
	second := topHeight(h.profilesForDecision(ctx, item, streams, dec))
	assert.Equal(t, 1080, first)
	assert.Equal(t, 480, second, "no freeze wired => each call re-derives against the current input")
}
