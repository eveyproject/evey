package transcoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Boot re-derives a persisted capability from its own Report, so anything Report
// drops is silently lost on every restart: decode evidence would fall back to
// output-rung pricing, and a persisted HW demotion would be permanent again.
func TestReportDerive_RoundTripsDecodeEvidenceAndDemotionClock(t *testing.T) {
	t.Parallel()
	demotedAt := time.Date(2026, 8, 4, 19, 16, 3, 0, time.UTC)
	original := EncodeCapability{
		SoftwareMaxHeight: 1080,
		SourceRate:        24,
		CPUSignature:      "sig",
		BenchSamples: []BenchRungSample{
			{Ladder: LadderSoftware, Height: 1080, FPS: 45.2, RequiredFPS: 28.8, Passing: true},
		},
		BenchSource: SourceClass{Codec: "hevc", Height: 2160},
		DecodeSamples: []DecodeRungSample{
			{Codec: "hevc", Height: 2160, FPS: 60},
			{Codec: "h264", Height: 2160, FPS: 240},
		},
		HWDemotedAt: demotedAt,
	}

	derived := Derive(original.Report(), DerivePolicy{Margin: 1.2})

	require.Len(t, derived.DecodeSamples, 2, "decode evidence must survive a boot re-derive")
	assert.Equal(t, original.DecodeSamples, derived.DecodeSamples)
	assert.Equal(t, original.BenchSource, derived.BenchSource, "the decode reference point must survive too")
	assert.Equal(t, demotedAt, derived.HWDemotedAt, "the demotion clock must survive, or the demotion is permanent again")
}
