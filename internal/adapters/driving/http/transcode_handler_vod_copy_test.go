package httpapi

// Tests for the copy-rung routing guard isCopyRequest, in particular the disk
// guard (fix c): the whole-file strategy duplicates ~the source size on disk, so
// a source that cannot fit must fall back to the segment-bounded transcode
// ladder instead of risking ENOSPC on a small box.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// copyEligibleStreams is one H.264 High@L4.0 8-bit video stream — the source a
// copy rung accepts (sourceCopyVideoCodec returns a non-empty avc1.640028).
func copyEligibleStreams() []*domainlibrary.MediaStream {
	w, hgt, level := 1920, 1080, 40
	return []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo,
		Codec:      "h264",
		Profile:    "High",
		Width:      &w,
		Height:     &hgt,
		Level:      &level,
		Keyframes:  []int64{0, 6000, 12000}, // keyframe-backfilled → copy-eligible
	}}
}

// newCopyTestHandler builds a minimal hlsVODServe with the copy path wired
// (copyProducer non-nil) and a real on-disk SegmentCache so FreeBytes() statfs
// returns the temp filesystem's real free space.
func newCopyTestHandler(t *testing.T) *hlsVODServe {
	t.Helper()
	cache, err := apptranscoding.NewSegmentCache(t.TempDir())
	require.NoError(t, err)
	// copyProducer only needs to be non-nil for isCopyRequest's "copy path wired"
	// gate; the disk guard reads cache.FreeBytes(), never the producer. A copy-mode
	// SoftwareProducer mirrors production (process/source nil — not exercised here).
	copyProd := apptranscoding.NewSoftwareProducer(cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	return &hlsVODServe{
		cache:        cache,
		copyProducer: copyProd,
	}
}

func TestIsCopyRequest_DiskGuard(t *testing.T) {
	h := newCopyTestHandler(t)
	dec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	streams := copyEligibleStreams()

	// A small source fits in the (large, real) temp filesystem → copy allowed.
	assert.True(t, h.isCopyRequest(dec, streams, 1<<20), "1MB source fits → copy rung")
	// Unknown size (0) → fail-open so behaviour matches the pre-guard path.
	assert.True(t, h.isCopyRequest(dec, streams, 0), "unknown size → fail-open allows copy")
	// A source larger than any real free space → guard refuses → transcode ladder.
	assert.False(t, h.isCopyRequest(dec, streams, 1<<60), "source can't fit on disk → fallback transcode")
}

func TestIsCopyRequest_NonCopyDecisionUnaffected(t *testing.T) {
	h := newCopyTestHandler(t)
	// A transcode decision is never a copy request, regardless of disk space.
	got := h.isCopyRequest(domaintranscoding.PlaybackDecision{VideoDecision: "transcode"}, copyEligibleStreams(), 1<<20)
	assert.False(t, got)
}

// Safety for the now-on-by-default rung: a copy-eligible codec/level but NO
// keyframe index (not yet keyframe-backfilled) must NOT route to copy — the plan
// would be empty and break playback. It falls back to transcode instead.
func TestIsCopyRequest_NoKeyframes_FallsBackToTranscode(t *testing.T) {
	h := newCopyTestHandler(t)
	dec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	w, hgt, level := 1920, 1080, 40
	unbackfilled := []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo,
		Codec:      "h264",
		Profile:    "High",
		Width:      &w,
		Height:     &hgt,
		Level:      &level,
		// Keyframes: nil — level is known but keyframes_ms not yet backfilled.
	}}
	assert.False(t, h.isCopyRequest(dec, unbackfilled, 1<<20),
		"eligible codec but no keyframe index → transcode, not copy (an empty plan would break)")
}

// Phase H1: the HEVC copy rung. Eligibility mirrors CopyRungEligible — the
// handler-level wiring (sourceCopyVideoCodec → CopySource projection) must
// thread the HDR ground-truth fields, or every HEVC title silently falls back
// to transcode (or worse, a DoVi P5 slips through).
func TestIsCopyRequest_HEVC(t *testing.T) {
	h := newCopyTestHandler(t)
	dec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	w, hgt, level := 3840, 2160, 153
	pq := "smpte2084"
	hevc := func(mutate func(*domainlibrary.MediaStream)) []*domainlibrary.MediaStream {
		s := &domainlibrary.MediaStream{
			StreamType:    domainlibrary.StreamTypeVideo,
			Codec:         "hevc",
			Profile:       "Main 10",
			Width:         &w,
			Height:        &hgt,
			Level:         &level,
			ColorTransfer: &pq,
			Keyframes:     []int64{0, 6000, 12000},
		}
		if mutate != nil {
			mutate(s)
		}
		return []*domainlibrary.MediaStream{s}
	}

	assert.True(t, h.isCopyRequest(dec, hevc(nil), 1<<20),
		"HEVC Main 10 with probed ground truth routes to copy")
	assert.False(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.ColorTransfer = nil }), 1<<20),
		"unprobed color_transfer gates the HEVC copy off")
	assert.False(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.DoViProfile = new(5) }), 1<<20),
		"DoVi profile 5 never routes to copy")
	assert.True(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.DoViProfile = new(8); s.DoViBLCompat = new(1) }), 1<<20),
		"DoVi 8.1 (HDR10-compatible base) routes to copy")
	assert.False(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.Keyframes = nil }), 1<<20),
		"no keyframe index → transcode (empty plan would break)")
}

func TestIsCopyRequest_ProducerOffNeverCopies(t *testing.T) {
	// copyProducer nil (EVEY_REMUX_COPY off) → never a copy request, even with an
	// eligible source and ample disk.
	h := &hlsVODServe{}
	got := h.isCopyRequest(domaintranscoding.PlaybackDecision{VideoDecision: "copy"}, copyEligibleStreams(), 1<<20)
	assert.False(t, got)
}

// Phase A2: passthrough eligibility + the codec the master must advertise.
// The two MUST stay coherent — a master claiming mp4a over ac-3 segments (or
// the reverse) is refused by Safari.
func TestAudioCopyEligibilityAndServedCodec(t *testing.T) {
	copyDec := domaintranscoding.PlaybackDecision{AudioDecision: "copy"}
	transDec := domaintranscoding.PlaybackDecision{AudioDecision: "transcode", AudioCodec: "aac"}

	cases := []struct {
		name      string
		dec       domaintranscoding.PlaybackDecision
		normalize bool
		profiles  int
		src       string
		eligible  bool
		served    string
	}{
		{"aac copy", copyDec, false, 1, "aac", true, "aac"},
		{"ac3 passthrough (Safari-class client)", copyDec, false, 1, "ac3", true, "ac3"},
		{"eac3 passthrough", copyDec, false, 1, "eac3", true, "eac3"},
		{"dts never passes through", copyDec, false, 1, "dts", false, ""},
		{"loudnorm forces re-encode", copyDec, true, 1, "ac3", false, ""},
		{"multi-rung ladder forces re-encode", copyDec, false, 3, "aac", false, ""},
		{"transcode verdict stays aac", transDec, false, 1, "ac3", false, "aac"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := audioCopyEligible(c.dec, c.normalize, c.profiles, c.src)
			if got != c.eligible {
				t.Fatalf("eligible = %v, want %v", got, c.eligible)
			}
			if served := servedAudioCodec(got, c.src, c.dec); served != c.served {
				t.Fatalf("servedAudioCodec = %q, want %q", served, c.served)
			}
		})
	}
}
