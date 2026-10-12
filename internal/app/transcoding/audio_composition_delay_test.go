package transcoding

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A/V desync on the ffmpeg-copy video fallback (prod, GIGN/Zombieland class).
//
// The audio rendition is served as its OWN stream, so it starts at 0. When the
// video is packaged by mkvgo the video also presents at 0 and the two agree.
// When mkvgo is not eligible (PlanHLS refuses the source) the video falls back
// to ffmpeg `-c:v copy` in fMP4, which presents its first frame LATE by
// has_b_frames/fps because MP4 forbids a negative DTS - 42 ms for 1 B-frame,
// 83 ms for 2 at 23.976 fps. The audio then runs EARLY by exactly that much,
// past the ~45 ms audibility threshold for early audio (ITU-R BT.1359).
//
// Proven live in prod: masters served with mkvgo_video=false AND a separate
// audio rendition (mkvgo_audio=true) - the exact unhandled combination the
// StartAudioOnly TODO described.
//
// The compensation exists end-to-end already (AudioProcessOpts.VideoCompositionDelay
// -> -output_ts_offset); what was missing is that NOTHING ever set it. These
// tests lock the two links that were absent: the fingerprint carries it, and the
// orchestrator hands it to every spawned session.
func TestAudioOrchestrator_SpawnsSessionWithVideoCompositionDelay(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.VideoCompositionDelay = 0.083 // 2 B-frames at 23.976 fps

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))

	opts := proc.optsAt(0)
	assert.InDelta(t, 0.083, opts.VideoCompositionDelay, 1e-9,
		"the audio session must be shifted onto the video's timeline, else the audio runs early and is audibly out of sync")
}

// A seeked session must carry the shift too: the desync is a property of the
// video packaging, not of where playback happens to start.
func TestAudioOrchestrator_SeekedSessionKeepsCompositionDelay(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.VideoCompositionDelay = 0.042

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 600))

	opts := proc.optsAt(0)
	assert.InDelta(t, 0.042, opts.VideoCompositionDelay, 1e-9,
		"a seeked session is shifted like a cold one")
}

// The shift changes the OUTPUT timestamps, so shifted and unshifted segments are
// NOT interchangeable bytes. It must therefore fork the cache: a title whose
// video packaging flips between mkvgo and the ffmpeg fallback must not serve
// segments minted under the other timeline.
func TestAudioFingerprint_CompositionDelayForksTheCache(t *testing.T) {
	base := minimalAudioFp()
	shifted := base
	shifted.VideoCompositionDelay = 0.083

	assert.NotEqual(t, base.Hash(), shifted.Hash(),
		"a shifted rendition must not alias the unshifted one - the bytes differ")

	same := base
	same.VideoCompositionDelay = 0
	assert.Equal(t, base.Hash(), same.Hash(),
		"no shift must stay byte-identical AND hash-identical to today's behaviour")
}
