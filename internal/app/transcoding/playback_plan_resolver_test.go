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

// wasm ON on the cast receiver rides ?wasmAudio=1 on EVERY cast - including
// media whose selected track is plain AAC. For those the server refuses the
// wasm master (nothing to decode client-side) and serves the NORMAL muxed
// master... but the fingerprint used to strip the audio track anyway
// (atrack=-1): master advertising audio over audio-free segments = CAF error
// 104/905 (prod, 2026-07-29). The strip must only apply when the selected
// track actually rides the wasm path (AC-3/E-AC-3).
func TestFingerprint_WasmAudioStripsAudioOnlyForDolbyTracks(t *testing.T) {
	pr := &PlanResolver{UseFMP4: true}

	req := resolverTestRequest()
	req.WasmAudio = true
	ch := 6
	req.Streams = append(req.Streams, &library.MediaStream{StreamType: library.StreamTypeAudio, Codec: "aac", Channels: &ch})
	plan, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 0, plan.Fingerprint.AudioTrack,
		"an AAC track cannot ride the wasm path: the muxed master needs its audio in the segments")

	// The same request on an AC-3 track: audio IS served out of band, the
	// video segments must be audio-free.
	reqAC3 := resolverTestRequest()
	reqAC3.WasmAudio = true
	reqAC3.Streams = append(reqAC3.Streams, &library.MediaStream{StreamType: library.StreamTypeAudio, Codec: "ac3", Channels: &ch})
	planAC3, err := pr.Resolve(context.Background(), reqAC3)
	require.NoError(t, err)
	assert.Equal(t, -1, planAC3.Fingerprint.AudioTrack,
		"an AC-3 track on the wasm path: segments are video-only")
}
