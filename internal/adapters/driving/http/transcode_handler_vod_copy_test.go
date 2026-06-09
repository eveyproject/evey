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
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil, 0))
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

func TestIsCopyRequest_ProducerOffNeverCopies(t *testing.T) {
	// copyProducer nil (EVEY_REMUX_COPY off) → never a copy request, even with an
	// eligible source and ample disk.
	h := &hlsVODServe{}
	got := h.isCopyRequest(domaintranscoding.PlaybackDecision{VideoDecision: "copy"}, copyEligibleStreams(), 1<<20)
	assert.False(t, got)
}
