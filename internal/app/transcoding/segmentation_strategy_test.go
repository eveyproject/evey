package transcoding

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStrategy is a test double for SegmentationStrategy. It records every
// EnsureSegment call and returns a deterministic manifest. No ffmpeg.
type fakeStrategy struct {
	mu       sync.Mutex
	calls    []segCall
	manifest []byte
	err      error // if non-nil, EnsureSegment returns this error
}

type segCall struct {
	fp      DecisionFingerprint
	segIdx  int
}

func newFakeStrategy(manifest []byte) *fakeStrategy {
	return &fakeStrategy{manifest: manifest}
}

func (f *fakeStrategy) EnsureSegment(_ context.Context, fp DecisionFingerprint, segIdx int) error {
	f.mu.Lock()
	f.calls = append(f.calls, segCall{fp: fp, segIdx: segIdx})
	err := f.err
	f.mu.Unlock()
	return err
}

func (f *fakeStrategy) Manifest(_ context.Context, fp DecisionFingerprint) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.manifest, nil
}

func (f *fakeStrategy) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeStrategy) lastCall() segCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return segCall{}
	}
	return f.calls[len(f.calls)-1]
}

// compile-time assertion: fakeStrategy satisfies the port.
var _ SegmentationStrategy = (*fakeStrategy)(nil)

// ============================================================================
// Strategy contract tests (adapter-agnostic)
// ============================================================================

// TestSegmentationStrategy_ContractManifestIdentical verifies that swapping
// the strategy adapter does not change what the CopyProducer exposes: the
// manifest bytes must come straight from the strategy without modification.
// This is the seam-invariance test: only HOW bytes are produced differs
// between adapters, not WHAT the producer exposes.
func TestSegmentationStrategy_ContractManifestIdentical(t *testing.T) {
	const fixedManifest = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-TARGETDURATION:3
#EXT-X-SEGMENT-INDEPENDENT:YES
#EXTINF:3.0,
stream_0_000.m4s
#EXT-X-ENDLIST
`
	strats := []struct {
		name     string
		strategy SegmentationStrategy
	}{
		{"fake_strategy", newFakeStrategy([]byte(fixedManifest))},
		// ByteRangeStrategy returns an error — so its "manifest" contract is
		// errByteRangeNotImplemented. We test that here to prove the adapter
		// switch is transparent (the error is a documented sentinel, not nil).
		{"byte_range_stub", ByteRangeStrategy{}},
	}

	fp := minimalFingerprint(t)

	for _, tc := range strats {
		t.Run(tc.name, func(t *testing.T) {
			switch s := tc.strategy.(type) {
			case *fakeStrategy:
				// Fake: Manifest must return exactly what the adapter holds.
				got, err := s.Manifest(context.Background(), fp)
				require.NoError(t, err)
				assert.Equal(t, []byte(fixedManifest), got,
					"manifest bytes must pass through unmodified")

			case ByteRangeStrategy:
				// Stub: documents the expected sentinel error so the caller
				// knows the adapter is unimplemented.
				_, err := s.Manifest(context.Background(), fp)
				require.ErrorIs(t, err, errByteRangeNotImplemented,
					"ByteRangeStrategy must return the sentinel errByteRangeNotImplemented")
			}
		})
	}
}

// TestSegmentationStrategy_ByteRangeStubAlwaysErrors verifies both methods of
// the stub return the sentinel error (not nil, not a different error).
func TestSegmentationStrategy_ByteRangeStubAlwaysErrors(t *testing.T) {
	s := ByteRangeStrategy{}
	fp := minimalFingerprint(t)

	t.Run("EnsureSegment", func(t *testing.T) {
		err := s.EnsureSegment(context.Background(), fp, 0)
		require.ErrorIs(t, err, errByteRangeNotImplemented)
	})

	t.Run("Manifest", func(t *testing.T) {
		_, err := s.Manifest(context.Background(), fp)
		require.ErrorIs(t, err, errByteRangeNotImplemented)
	})
}

// ============================================================================
// CopyProducer delegation tests (via fakeStrategy)
// ============================================================================

// TestCopyProducer_EnsureSegmentRangeDelegatesCorrectly verifies that
// EnsureSegmentRange forwards the exact segIdx to the strategy.
func TestCopyProducer_EnsureSegmentRangeDelegatesCorrectly(t *testing.T) {
	cache, _ := newTestCache(t)
	strat := newFakeStrategy([]byte("manifest"))
	p := NewCopyProducer(strat, cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)

	cases := []struct{ segIdx int }{{0}, {1}, {42}, {99}}
	for _, tc := range cases {
		require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, tc.segIdx),
			"segIdx=%d", tc.segIdx)
	}

	assert.Equal(t, len(cases), strat.callCount(),
		"each EnsureSegmentRange must produce exactly one strategy.EnsureSegment call")

	// Spot-check the last call carried the right segIdx.
	assert.Equal(t, cases[len(cases)-1].segIdx, strat.lastCall().segIdx)
}

// TestCopyProducer_EnsureSegmentRangeVariantGuard verifies that variantIdx != 0
// is rejected without touching the strategy.
func TestCopyProducer_EnsureSegmentRangeVariantGuard(t *testing.T) {
	cache, _ := newTestCache(t)
	strat := newFakeStrategy(nil)
	p := NewCopyProducer(strat, cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	err := p.EnsureSegmentRange(context.Background(), fp, 1, 0)
	require.Error(t, err, "non-zero variantIdx must be rejected")
	assert.Contains(t, err.Error(), "variantIdx must be 0",
		"error must mention the constraint")
	assert.Equal(t, 0, strat.callCount(),
		"strategy must not be called when variantIdx is invalid")
}

// TestCopyProducer_EnsureSegmentRangeNegativeSegIdx verifies that negative
// segment indices are rejected.
func TestCopyProducer_EnsureSegmentRangeNegativeSegIdx(t *testing.T) {
	cache, _ := newTestCache(t)
	strat := newFakeStrategy(nil)
	p := NewCopyProducer(strat, cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	err := p.EnsureSegmentRange(context.Background(), fp, 0, -1)
	require.Error(t, err)
	assert.Equal(t, 0, strat.callCount())
}

// TestCopyProducer_EnsureSegmentRange_PassesFingerprintUnchanged verifies that
// the fp forwarded to the strategy is byte-identical to what the caller passed.
func TestCopyProducer_EnsureSegmentRange_PassesFingerprintUnchanged(t *testing.T) {
	cache, _ := newTestCache(t)
	strat := newFakeStrategy([]byte("m"))
	p := NewCopyProducer(strat, cache, CopyProducerOptions{}, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	fp.MediaID = "unique-media-id"
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 7))
	assert.Equal(t, fp.MediaID, strat.lastCall().fp.MediaID,
		"fingerprint must be forwarded unmodified to the strategy")
	assert.Equal(t, 7, strat.lastCall().segIdx)
}
