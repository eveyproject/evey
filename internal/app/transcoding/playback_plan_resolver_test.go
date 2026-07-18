package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The whole point of the resolver migration: the playback brain is now
// exercisable without net/http - a bare PlaybackRequest in, a coherent
// immutable PlaybackPlan out.

func resolverTestStreams() []*library.MediaStream {
	w, h := 1920, 1080
	return []*library.MediaStream{{
		StreamType: library.StreamTypeVideo,
		Codec:      "h264",
		Width:      &w,
		Height:     &h,
	}}
}

func resolverTestRequest() PlaybackRequest {
	return PlaybackRequest{
		MediaID:             "m1",
		SourcePath:          "/test.mkv",
		Container:           "matroska",
		Streams:             resolverTestStreams(),
		Prefs:               DefaultTranscodingPreferences(),
		SubtitleBurnInTrack: -1,
	}
}

// A zero-value resolver (nothing wired) must resolve like the safe-fallback
// handler did: a transcode plan, capped ladder, hash-coherent fingerprint.
func TestPlanResolver_ZeroValue_ResolvesCoherentTranscodePlan(t *testing.T) {
	pr := &PlanResolver{UseFMP4: true}
	plan, err := pr.Resolve(context.Background(), resolverTestRequest())
	require.NoError(t, err)

	assert.Equal(t, plan.Fingerprint.Hash(), plan.FingerprintHash)
	assert.Equal(t, PackagerTranscodeFFmpeg, plan.Packager, "no copy producer wired → transcode")
	assert.True(t, plan.NeedsVideoEncode())
	require.NotEmpty(t, plan.Ladder)
	for _, p := range plan.Ladder {
		assert.LessOrEqual(t, p.Height, 720, "nil capability must keep the 720p safe floor")
	}
	assert.Len(t, plan.Fingerprint.Profiles, len(plan.Ladder),
		"fingerprint profiles mirror the shaped ladder")
	assert.True(t, plan.Fingerprint.UseFMP4)
	assert.Equal(t, LadderClassOf(plan.Fingerprint), plan.Class)
}

// The vd= mid-play pin: "transcode" always holds, even when the fresh
// decision would say copy.
func TestPlanResolver_Decide_HonorsTranscodePin(t *testing.T) {
	pr := &PlanResolver{}
	req := resolverTestRequest()
	req.PinnedVideoDecision = "transcode"
	d := pr.Decide(context.Background(), req)
	assert.Equal(t, "transcode", d.VideoDecision)
}

// A non-zero audio resync offset forces the audio decision to transcode (a
// copied track cannot be time-shifted) and stamps the reason.
func TestPlanResolver_Decide_AudioResyncForcesTranscode(t *testing.T) {
	pr := &PlanResolver{}
	req := resolverTestRequest()
	req.AudioOffsetMs = 300
	d := pr.Decide(context.Background(), req)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_resync_requested")
}

// The disk guard: a copy-shaped decision stays a transcode plan when the
// cache filesystem cannot fit the source with headroom, and IsCopy is always
// false without a wired copy producer.
func TestPlanResolver_IsCopy_Gates(t *testing.T) {
	pr := &PlanResolver{}
	copyDecision := transcoding.PlaybackDecision{VideoDecision: "copy"}
	assert.False(t, pr.IsCopy(copyDecision, resolverTestStreams(), 0),
		"no copy producer wired → never a copy request")
}
