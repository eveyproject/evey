package transcoding

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// source_facts.go had NO test file, and mutation testing found 21 survivors in
// it. These are the functions that decide whether a stream can ride a copy rung,
// which audio codec the segments actually carry, and whether the source is HDR -
// i.e. the facts every playback decision is built on. Their own comments warn
// that a disagreement here makes the master advertise a codec the segments do
// not carry, and Safari then refuses to play.

func videoStream(mutate func(*library.MediaStream)) *library.MediaStream {
	s := &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: "h264"}
	if mutate != nil {
		mutate(s)
	}
	return s
}

func TestSourceCopyVideoRange(t *testing.T) {
	sdr := videoStream(func(s *library.MediaStream) { s.ColorTransfer = new("bt709") })
	if got := SourceCopyVideoRange([]*library.MediaStream{sdr}); got != "" {
		t.Errorf("SDR must advertise no VIDEO-RANGE, got %q", got)
	}

	hlg := videoStream(func(s *library.MediaStream) { s.ColorTransfer = new("arib-std-b67") })
	if got := SourceCopyVideoRange([]*library.MediaStream{hlg}); got != "HLG" {
		t.Errorf("HLG source = %q, want HLG", got)
	}

	pq := videoStream(func(s *library.MediaStream) { s.ColorTransfer = new("smpte2084") })
	if got := SourceCopyVideoRange([]*library.MediaStream{pq}); got != "PQ" {
		t.Errorf("PQ source = %q, want PQ", got)
	}

	// No video stream at all, and a nil entry in the slice, must both be
	// survivable - a degenerate probe must not panic the manifest builder.
	if got := SourceCopyVideoRange(nil); got != "" {
		t.Errorf("no streams = %q, want empty", got)
	}
	audioOnly := []*library.MediaStream{nil, {StreamType: library.StreamTypeAudio, Codec: "aac"}}
	if got := SourceCopyVideoRange(audioOnly); got != "" {
		t.Errorf("audio-only = %q, want empty", got)
	}
}

// The keyframe guard is the one with teeth: the copy plan (segment cuts and
// StartSec) is built from s.Keyframes, so an item not yet backfilled MUST fall
// back to transcode rather than route to copy and break on an empty plan.
func TestSourceCopyVideoCodec_RequiresAKeyframeIndex(t *testing.T) {
	level := 40
	noKeyframes := videoStream(func(s *library.MediaStream) {
		s.Profile, s.Level = "High", &level
	})
	if got := SourceCopyVideoCodec([]*library.MediaStream{noKeyframes}); got != "" {
		t.Errorf("a stream without a keyframe index must not be copy-advertised, got %q", got)
	}

	withKeyframes := videoStream(func(s *library.MediaStream) {
		s.Profile, s.Level = "High", &level
		s.Keyframes = []int64{0, 2000, 4000}
	})
	if got := SourceCopyVideoCodec([]*library.MediaStream{withKeyframes}); got == "" {
		t.Error("a keyframe-indexed h264 High@4.0 stream should yield a codec string")
	}
}

func TestVideoCopyEligible(t *testing.T) {
	level := 40
	ok := videoStream(func(s *library.MediaStream) {
		s.Profile, s.Level, s.Keyframes = "High", &level, []int64{0, 2000}
	})
	streams := []*library.MediaStream{ok}

	if !VideoCopyEligible(domaintranscoding.PlaybackDecision{VideoDecision: "copy"}, streams) {
		t.Error("copy decision + copyable source must be eligible")
	}
	// Both halves matter independently.
	if VideoCopyEligible(domaintranscoding.PlaybackDecision{VideoDecision: "transcode"}, streams) {
		t.Error("a transcode decision must never be copy-eligible")
	}
	noKf := videoStream(func(s *library.MediaStream) { s.Profile, s.Level = "High", &level })
	if VideoCopyEligible(domaintranscoding.PlaybackDecision{VideoDecision: "copy"}, []*library.MediaStream{noKf}) {
		t.Error("a copy decision on a non-copyable source must not be eligible")
	}
}

// The CLIENT-BLIND baseline, still the gate the mkvgo CMAF packager reads.
// Widening it would hand that packager a codec nobody proved it can carry.
func TestAudioCopyCodecAllowed(t *testing.T) {
	for _, c := range []string{"aac", "ac3", "eac3"} {
		if !AudioCopyCodecAllowed(c) {
			t.Errorf("%s must be passthrough-allowed", c)
		}
	}
	// Anything outside the wired fMP4 + RFC 6381 set must be re-encoded.
	for _, c := range []string{"flac", "opus", "mp3", "dts", "truehd", "", "AAC"} {
		if AudioCopyCodecAllowed(c) {
			t.Errorf("%q must not be passthrough-allowed", c)
		}
	}
}

// The client-aware gate. An opted-in client unlocks mp3 and NOTHING else: the
// declaration selects from fmp4PassthroughOptIn, it does not extend it.
func TestAudioCopyCodecAllowedFor_OptIn(t *testing.T) {
	none := domaintranscoding.ClientProfile{}
	mp3OK := domaintranscoding.ClientProfile{FMP4AudioPassthrough: []string{"mp3"}}

	if AudioCopyCodecAllowedFor("mp3", none) {
		t.Error("mp3 must stay re-encoded for a client that did not opt in")
	}
	if !AudioCopyCodecAllowedFor("mp3", mp3OK) {
		t.Error("an opted-in client must get the mp3 passthrough")
	}
	// The baseline is unconditional: opting in changes nothing for aac/ac3/eac3.
	for _, c := range []string{"aac", "ac3", "eac3"} {
		if !AudioCopyCodecAllowedFor(c, none) {
			t.Errorf("%s must stay allowed with no declaration", c)
		}
	}
	// A stale or hostile declaration can only ever pick from the opt-in set.
	for _, c := range []string{"dts", "truehd", "flac", "opus"} {
		greedy := domaintranscoding.ClientProfile{FMP4AudioPassthrough: []string{c}}
		if AudioCopyCodecAllowedFor(c, greedy) {
			t.Errorf("%q must not be unlockable by a client declaration", c)
		}
	}
	// Codec-name aliases fold on both sides, as everywhere else in the decision.
	if !AudioCopyCodecAllowedFor("MP3", domaintranscoding.ClientProfile{FMP4AudioPassthrough: []string{"mp3"}}) {
		t.Error("the source codec spelling must be normalised")
	}
	if !AudioCopyCodecAllowedFor("mp3", domaintranscoding.ClientProfile{FMP4AudioPassthrough: []string{"MP3"}}) {
		t.Error("the client's declared spelling must be normalised")
	}
}

// Four independent gates; each one alone must be able to refuse. This is the
// single source of truth for both the fingerprint and the manifest, so a gate
// that stops working makes the two disagree.
func TestAudioCopyEligible_EachGateRefusesAlone(t *testing.T) {
	copyDecision := domaintranscoding.PlaybackDecision{AudioDecision: "copy"}
	anyClient := domaintranscoding.ClientProfile{}

	if !AudioCopyEligible(copyDecision, false, 1, "aac", anyClient) {
		t.Fatal("the all-clear case must be eligible")
	}
	if AudioCopyEligible(domaintranscoding.PlaybackDecision{AudioDecision: "transcode"}, false, 1, "aac", anyClient) {
		t.Error("a transcode decision must refuse")
	}
	if AudioCopyEligible(copyDecision, true, 1, "aac", anyClient) {
		t.Error("loudness normalisation requires a re-encode")
	}
	if AudioCopyEligible(copyDecision, false, 2, "aac", anyClient) {
		t.Error("a multi-rung ladder always re-encodes")
	}
	if AudioCopyEligible(copyDecision, false, 1, "flac", anyClient) {
		t.Error("a codec outside the passthrough set must refuse")
	}
	if AudioCopyEligible(copyDecision, false, 1, "mp3", anyClient) {
		t.Error("mp3 without the client opt-in must refuse")
	}
}

func TestServedAudioCodec(t *testing.T) {
	dec := domaintranscoding.PlaybackDecision{AudioCodec: "aac"}
	if got := ServedAudioCodec(true, "eac3", dec); got != "eac3" {
		t.Errorf("on passthrough the SOURCE codec is served, got %q", got)
	}
	if got := ServedAudioCodec(false, "eac3", dec); got != "aac" {
		t.Errorf("without passthrough the decision's encoder is served, got %q", got)
	}
}

// Both index helpers count RELATIVE to their own stream type (ffmpeg's 0:a:N /
// 0:s:N), not the absolute slice position - getting that wrong serves the wrong
// track.
func TestSourceAudioAndSubtitleCodec_IndexRelativeToType(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: library.StreamTypeVideo, Codec: "h264"},
		{StreamType: library.StreamTypeAudio, Codec: "AAC"},
		{StreamType: library.StreamTypeSubtitle, Codec: "SUBRIP"},
		{StreamType: library.StreamTypeAudio, Codec: "EAC3"},
		{StreamType: library.StreamTypeSubtitle, Codec: "PGS"},
	}

	if got := SourceAudioCodec(streams, 0); got != "aac" {
		t.Errorf("audio 0 = %q, want aac (lowercased)", got)
	}
	if got := SourceAudioCodec(streams, 1); got != "eac3" {
		t.Errorf("audio 1 = %q, want eac3 - the second AUDIO track, not slice index 1", got)
	}
	if got := SourceAudioCodec(streams, 2); got != "" {
		t.Errorf("out-of-range audio index = %q, want empty", got)
	}

	if got := SourceSubtitleCodec(streams, 1); got != "pgs" {
		t.Errorf("subtitle 1 = %q, want pgs", got)
	}
	// A negative index means "no burn-in track" and must not index anything.
	if got := SourceSubtitleCodec(streams, -1); got != "" {
		t.Errorf("negative subtitle index = %q, want empty", got)
	}
	if got := SourceSubtitleCodec(streams, 5); got != "" {
		t.Errorf("out-of-range subtitle index = %q, want empty", got)
	}
}

func TestSourceDeinterlaceAll(t *testing.T) {
	yes, no := true, false
	if !SourceDeinterlaceAll([]*library.MediaStream{videoStream(func(s *library.MediaStream) { s.IdetInterlaced = &yes })}) {
		t.Error("an idet verdict of interlaced must request deint=all")
	}
	if SourceDeinterlaceAll([]*library.MediaStream{videoStream(func(s *library.MediaStream) { s.IdetInterlaced = &no })}) {
		t.Error("an explicit not-interlaced verdict must not force deint")
	}
	// Not analysed yet: fall back to the flag-gated path, never force.
	if SourceDeinterlaceAll([]*library.MediaStream{videoStream(nil)}) {
		t.Error("a nil idet verdict must not force deint")
	}
	if SourceDeinterlaceAll(nil) {
		t.Error("no streams must not force deint")
	}
}

func TestSourceSAR(t *testing.T) {
	anamorphic := videoStream(func(s *library.MediaStream) { s.SampleAspectRatio = new("64:45") })
	if got := SourceSAR([]*library.MediaStream{anamorphic}); got != "64:45" {
		t.Errorf("SAR = %q, want 64:45", got)
	}
	if got := SourceSAR([]*library.MediaStream{videoStream(nil)}); got != "" {
		t.Errorf("an unprobed SAR = %q, want empty", got)
	}
	if got := SourceSAR(nil); got != "" {
		t.Errorf("no streams = %q, want empty", got)
	}
}

// The two ten-bit rules must agree across layers: the probed depth OR the
// profile name, because ffprobe leaves bits_per_raw_sample empty on many HEVC
// rips.
func TestStreamIsTenBit(t *testing.T) {
	eight, ten := 8, 10
	if StreamIsTenBit(nil) {
		t.Error("a nil stream is not ten-bit")
	}
	if !StreamIsTenBit(&library.MediaStream{BitDepth: &ten}) {
		t.Error("a probed depth of 10 is ten-bit")
	}
	if StreamIsTenBit(&library.MediaStream{BitDepth: &eight}) {
		t.Error("a probed depth of 8 is not ten-bit")
	}
	if !StreamIsTenBit(&library.MediaStream{Profile: "Main 10"}) {
		t.Error("a Main 10 profile is ten-bit even with no probed depth")
	}
	if !StreamIsTenBit(&library.MediaStream{Profile: "High 10"}) {
		t.Error("a High 10 profile is ten-bit")
	}
	if StreamIsTenBit(&library.MediaStream{Profile: "High"}) {
		t.Error("a plain High profile is not ten-bit")
	}
}

func TestSourceVideoCodecAndTenBitAndDims(t *testing.T) {
	w, h, ten := 1920, 1080, 10
	streams := []*library.MediaStream{
		nil,
		{StreamType: library.StreamTypeAudio, Codec: "aac"},
		{StreamType: library.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &h, BitDepth: &ten},
	}

	if got := SourceVideoCodec(streams); got != "hevc" {
		t.Errorf("SourceVideoCodec = %q, want hevc (skipping nil and audio)", got)
	}
	if !SourceVideoTenBit(streams) {
		t.Error("a 10-bit video stream must report ten-bit")
	}

	dims := SourceDimsOf(streams)
	if dims.Width != 1920 || dims.Height != 1080 || dims.Codec != "hevc" || !dims.TenBit {
		t.Errorf("SourceDimsOf = %+v, want 1920x1080 hevc ten-bit", dims)
	}

	// A missing probe must stay "unknown" (zero), not be conflated with a real
	// zero-pixel value - CapLadderToSource reads zero as "don't cap".
	unprobed := SourceDimsOf([]*library.MediaStream{{StreamType: library.StreamTypeVideo, Codec: "h264"}})
	if unprobed.Width != 0 || unprobed.Height != 0 {
		t.Errorf("an unprobed stream must yield zero dims, got %+v", unprobed)
	}
	if got := SourceVideoCodec(nil); got != "" {
		t.Errorf("no streams = %q, want empty", got)
	}
	if SourceVideoTenBit(nil) {
		t.Error("no streams must not report ten-bit")
	}
}
