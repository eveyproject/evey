package transcoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// Compile-time assertion: CopyProducer satisfies the StreamProducer seam.
// If the interface changes and CopyProducer stops satisfying it, this file
// fails to compile — the test suite then fails without a confusing runtime
// panic.
var _ StreamProducer = (*CopyProducer)(nil)

// ============================================================================
// Profiles determinism
// ============================================================================

// TestCopyProducer_ProfilesDeterministic verifies that identical inputs always
// produce the same ladder. This property is required because Profiles() feeds
// DecisionFingerprint.Hash() at both manifest-time and segment-time; any
// non-determinism forks the cache forever.
func TestCopyProducer_ProfilesDeterministic(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewCopyProducer(newFakeStrategy(nil), cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	src := SourceDims{Width: 1920, Height: 1080, Codec: "h264"}
	decision := transcoding.PlaybackDecision{VideoDecision: "copy"}
	cap := transcoding.EncodeCapability{}

	a := p.Profiles(nil, src, decision, cap)
	b := p.Profiles(nil, src, decision, cap)

	require.Len(t, a, 1, "copy producer must expose exactly one rung")
	assert.Equal(t, a, b, "Profiles must be deterministic for the same inputs")
}

// TestCopyProducer_ProfilesSingleRung verifies that Profiles always returns
// exactly one variant regardless of the base ladder passed in. The base ladder
// is intentionally ignored for copy — no ABR ladder exists.
func TestCopyProducer_ProfilesSingleRung(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewCopyProducer(newFakeStrategy(nil), cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	base := []transcoding.HLSProfile{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2500, AudioBitrate: 128},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
	}
	src := SourceDims{Width: 1920, Height: 1080, Codec: "h264"}

	profiles := p.Profiles(base, src, transcoding.PlaybackDecision{}, transcoding.EncodeCapability{})
	require.Len(t, profiles, 1,
		"copy producer ignores the base ladder and always returns ONE rung (source dims)")
}

// TestCopyProducer_ProfilesSourceResolution verifies that the single rung's
// resolution matches the source (the copy path never re-scales).
func TestCopyProducer_ProfilesSourceResolution(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewCopyProducer(newFakeStrategy(nil), cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	cases := []SourceDims{
		{Width: 1280, Height: 720, Codec: "h264"},
		{Width: 3840, Height: 2160, Codec: "h264"},
		{Width: 640, Height: 480, Codec: "h264"},
	}
	for _, src := range cases {
		rungs := p.Profiles(nil, src, transcoding.PlaybackDecision{}, transcoding.EncodeCapability{})
		require.Len(t, rungs, 1)
		assert.Equal(t, src.Width, rungs[0].Width,
			"rung width must match source for src %dx%d", src.Width, src.Height)
		assert.Equal(t, src.Height, rungs[0].Height,
			"rung height must match source for src %dx%d", src.Width, src.Height)
	}
}

// TestCopyProducer_ProfilesZeroDimsDefaultsGracefully verifies that a zero-
// dimension source (metadata not yet available) produces a valid rung rather
// than a zero-dimension entry that would make ffmpeg abort.
func TestCopyProducer_ProfilesZeroDimsDefaultsGracefully(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewCopyProducer(newFakeStrategy(nil), cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	rungs := p.Profiles(nil, SourceDims{}, transcoding.PlaybackDecision{}, transcoding.EncodeCapability{})
	require.Len(t, rungs, 1)
	assert.Greater(t, rungs[0].Width, 0, "zero-dim source must produce a positive-width rung")
	assert.Greater(t, rungs[0].Height, 0, "zero-dim source must produce a positive-height rung")
}

// ============================================================================
// WaitBudget
// ============================================================================

func TestCopyProducer_WaitBudgetDefault(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewCopyProducer(newFakeStrategy(nil), cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	assert.GreaterOrEqual(t, p.WaitBudget(), time.Minute,
		"default wait budget must be at least 1 min (whole-file mux can be slow)")
}

func TestCopyProducer_WaitBudgetCustom(t *testing.T) {
	cache, _ := newTestCache(t)
	custom := 2 * time.Minute
	p := NewCopyProducer(newFakeStrategy(nil), cache, CopyProducerOptions{WaitBudgetDur: custom}, discardLogger())
	defer p.Close()

	assert.Equal(t, custom, p.WaitBudget())
}

// ============================================================================
// ActiveSessionCount / RemoveSession (no-op contracts)
// ============================================================================

func TestCopyProducer_ActiveSessionCountAlwaysZero(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewCopyProducer(newFakeStrategy(nil), cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	assert.Equal(t, 0, p.ActiveSessionCount(),
		"copy producer does not track per-viewer sessions — always 0")
}

func TestCopyProducer_RemoveSessionNoOp(t *testing.T) {
	cache, _ := newTestCache(t)
	strat := newFakeStrategy(nil)
	p := NewCopyProducer(strat, cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	// Must not panic and must not delegate to the strategy.
	p.RemoveSession("media-abc")
	assert.Equal(t, 0, strat.callCount(),
		"RemoveSession must not call the strategy")
}

// ============================================================================
// Close delegates to strategy when it is closeable
// ============================================================================

type closeableStrategy struct {
	*fakeStrategy
	closed bool
}

func (c *closeableStrategy) Close() { c.closed = true }

func TestCopyProducer_ClosePropagatesWhenStrategyIsCloseable(t *testing.T) {
	cache, _ := newTestCache(t)
	strat := &closeableStrategy{fakeStrategy: newFakeStrategy(nil)}
	p := NewCopyProducer(strat, cache, CopyProducerOptions{}, discardLogger())

	p.Close()
	assert.True(t, strat.closed, "Close must propagate to closeable strategy")
}

func TestCopyProducer_CloseNoOpWhenStrategyNotCloseable(t *testing.T) {
	cache, _ := newTestCache(t)
	// ByteRangeStrategy has no Close method — must not panic.
	p := NewCopyProducer(ByteRangeStrategy{}, cache, CopyProducerOptions{}, discardLogger())
	require.NotPanics(t, func() { p.Close() })
}

// ============================================================================
// variantIdx guard (comprehensive)
// ============================================================================

func TestCopyProducer_VariantGuardAllNonZero(t *testing.T) {
	cache, _ := newTestCache(t)
	strat := newFakeStrategy(nil)
	p := NewCopyProducer(strat, cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	for _, idx := range []int{1, 2, 10, 100} {
		err := p.EnsureSegmentRange(nil, fp, idx, 0) //nolint:staticcheck // intentional nil ctx test
		require.Error(t, err, "variantIdx=%d must be rejected", idx)
	}
	assert.Equal(t, 0, strat.callCount(),
		"strategy must never be reached for non-zero variantIdx")
}
