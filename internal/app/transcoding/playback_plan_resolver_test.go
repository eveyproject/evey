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
	assert.False(t, plan.Route.Copy, "no copy producer wired → transcode")
	assert.Equal(t, DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, plan.Delivery, "no packager wired → ffmpeg carries both tracks")
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

// ============================================================================
// HDR tonemap re-add: a transcoded rung is SDR BY CONTRACT
// ============================================================================

// hdrHEVCStreams is a PQ HEVC Main10 source, keyframe-indexed and levelled so
// it is copy-eligible - the only shape where the re-add must NOT fire.
func hdrHEVCStreams() []*library.MediaStream {
	w, h := 3840, 2160
	level, depth := 153, 10
	pq := "smpte2084"
	return []*library.MediaStream{{
		StreamType:    library.StreamTypeVideo,
		Codec:         "hevc",
		Profile:       "Main 10",
		Level:         &level,
		Width:         &w,
		Height:        &h,
		BitDepth:      &depth,
		ColorTransfer: &pq,
		Keyframes:     []int64{0, 6000, 12000},
	}}
}

func hdrRequest() PlaybackRequest {
	req := resolverTestRequest()
	req.Streams = hdrHEVCStreams()
	return req
}

// hdrCapableClientRequest is the shape the re-add exists for: a client that
// decodes the source AND advertises HDR, so the decision-level tonemap is
// legitimately suppressed. Whether the tonemap comes back then depends purely
// on whether the copy rung really serves it.
func hdrCapableClientRequest() PlaybackRequest {
	req := hdrRequest()
	req.Container = "mp4"
	req.Client = transcoding.ClientProfile{
		VideoCodecs: []string{"h264", "hevc"},
		AudioCodecs: []string{"aac"},
		Containers:  []string{"mp4"},
		SupportsHDR: true,
	}
	return req
}

// The master only advertises VIDEO-RANGE on the COPY rung, so an HDR source
// that ends up RE-ENCODED must be tone-mapped. An hdr=1 client legitimately
// suppresses the decision-level tonemap for the raw-serve path; when the copy
// rung is unavailable that suppression left a PQ source x264-encoded without
// tonemapping, which renders washed out (SATAVG 3.1 on prod bytes).
func TestPlanResolver_Decide_ReAddsTonemapWhenHDRWillBeReEncoded(t *testing.T) {
	pr := &PlanResolver{} // no copy producer wired -> this request re-encodes

	// The hdr=1 client suppressed the decision-level tonemap; with no copy rung
	// to carry the source VIDEO-RANGE, the re-add is the only thing left.
	d := pr.Decide(context.Background(), hdrCapableClientRequest())

	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed",
		"an HDR source that will not be served as a raw copy must be tone-mapped")
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.NotEmpty(t, d.VideoCodec, "a forced transcode must name the output codec")
}

// The mirror image: when the copy rung really serves the source bytes, the
// tonemap must NOT be re-added - the rung carries the source's own VIDEO-RANGE
// and tone-mapping a raw copy is not even expressible.
func TestPlanResolver_Decide_LeavesARawCopyUntonemapped(t *testing.T) {
	pr := &PlanResolver{CopyProducer: &stubProducer{}}

	d := pr.Decide(context.Background(), hdrCapableClientRequest())

	require.Equal(t, "copy", d.VideoDecision,
		"premise: an HDR-capable client on a copy-eligible source must reach the raw-copy path (reasons: %v)",
		d.TranscodeReasons)
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed",
		"a raw copy rung carries the source VIDEO-RANGE - re-adding a tonemap would be a lie")
}

// An SDR source must never acquire a tonemap it does not need: the re-add is
// gated on the source really being HDR, not on the absence of a copy rung.
func TestPlanResolver_Decide_NeverTonemapsAnSDRSource(t *testing.T) {
	pr := &PlanResolver{}

	d := pr.Decide(context.Background(), resolverTestRequest())

	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

// The ad= audio pin has the same contract as vd=: "transcode" always holds,
// because a re-encode is always serveable.
func TestPlanResolver_Decide_HonorsAudioTranscodePin(t *testing.T) {
	pr := &PlanResolver{}
	req := resolverTestRequest()
	req.PinnedAudioDecision = "transcode"

	d := pr.Decide(context.Background(), req)

	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "decision_pinned")
	assert.NotEmpty(t, d.AudioCodec, "a forced audio transcode must name the output codec")
}

// What stream-info announces is what the master serves. A decision that says
// "copy" on the codecs while the copy rung is gated (here: no keyframe index,
// and no producer wired) is reported as the transcode it will be, with the
// gate named - the tooltip must not say "copy" over an H.264 ladder.
func TestPlanResolver_Reported_NamesTheClosedCopyGate(t *testing.T) {
	pr := &PlanResolver{UseFMP4: true}
	req := resolverTestRequest()
	req.Client = transcoding.ClientProfile{VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}, Containers: []string{"mp4", "webm"}}
	// The pure decision copies the video: the client decodes h264.
	pure := pr.Decide(context.Background(), req)
	require.Equal(t, "copy", pure.VideoDecision)

	d := pr.Reported(context.Background(), req)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Contains(t, d.TranscodeReasons, "copy_blocked:no_keyframes")
	assert.NotEmpty(t, d.VideoCodec, "a transcode names its encoder")

	// A direct play (mp4 to an mp4 client) is the raw file: no rung is
	// involved and the report stays a direct play.
	req.Container = "mp4"
	direct := pr.Reported(context.Background(), req)
	assert.Equal(t, "direct_play", direct.PlayMethod)
	assert.Equal(t, "copy", direct.VideoDecision)
	assert.Empty(t, direct.TranscodeReasons)
}
