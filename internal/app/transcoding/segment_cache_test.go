package transcoding

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// DecisionFingerprint.Hash
// ============================================================================

func TestDecisionFingerprint_Hash_IsStable(t *testing.T) {
	// Same fingerprint twice ⇒ same hash. Trivial but a precondition for
	// addressing the cache by it.
	fp := minimalFingerprint(t)
	assert.Equal(t, fp.Hash(), fp.Hash())
}

func TestDecisionFingerprint_Hash_ProfileOrderIndependent(t *testing.T) {
	// The orchestrator hashes from the canonical (sorted by width) form, so
	// callers pushing profiles in any order hit the same cache.
	fpA := minimalFingerprint(t)
	fpA.Profiles = []ProfileFingerprint{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
		{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400, AudioBitrate: 128},
	}
	fpB := minimalFingerprint(t)
	fpB.Profiles = []ProfileFingerprint{
		{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400, AudioBitrate: 128},
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
	}
	assert.Equal(t, fpA.Hash(), fpB.Hash())
}

func TestDecisionFingerprint_Hash_DistinguishesAudioTrack(t *testing.T) {
	// Two decisions that differ on audio track MUST land in separate caches —
	// otherwise the wrong-audio segment would be served to one of them.
	fpA := minimalFingerprint(t)
	fpA.AudioTrack = 0
	fpB := minimalFingerprint(t)
	fpB.AudioTrack = 1
	assert.NotEqual(t, fpA.Hash(), fpB.Hash())
}

func TestDecisionFingerprint_Hash_DistinguishesContainer(t *testing.T) {
	// fMP4 segments are byte-incompatible with TS — must hash to different keys.
	fpA := minimalFingerprint(t)
	fpA.UseFMP4 = false
	fpB := minimalFingerprint(t)
	fpB.UseFMP4 = true
	assert.NotEqual(t, fpA.Hash(), fpB.Hash())
}

// ============================================================================
// SegmentKey.fileName
// ============================================================================

func TestSegmentKey_FileName(t *testing.T) {
	cases := []struct {
		name    string
		k       SegmentKey
		fmp4    bool
		expect  string
	}{
		{"ts variant 0 seg 0", SegmentKey{VariantIdx: 0, SegmentIdx: 0}, false, "stream_0_000.ts"},
		{"ts variant 1 seg 42", SegmentKey{VariantIdx: 1, SegmentIdx: 42}, false, "stream_1_042.ts"},
		{"fmp4 variant 2 seg 7", SegmentKey{VariantIdx: 2, SegmentIdx: 7}, true, "stream_2_007.m4s"},
		{"fmp4 init variant 0", SegmentKey{VariantIdx: 0, IsInit: true}, true, "stream_0_init.mp4"},
		{"fmp4 init variant 3", SegmentKey{VariantIdx: 3, IsInit: true}, true, "stream_3_init.mp4"},
		{"ts init still .mp4 — init is always fMP4", SegmentKey{VariantIdx: 0, IsInit: true}, false, "stream_0_init.mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, tc.k.fileName(tc.fmp4))
		})
	}
}

// ============================================================================
// SegmentCache.Path, IsReady, Wait, CleanupHash
// ============================================================================

func TestSegmentCache_Path_IsDeterministic(t *testing.T) {
	c, dir := newTestCache(t)
	k := SegmentKey{DecisionHash: "deadbeef", VariantIdx: 0, SegmentIdx: 5}
	expect := filepath.Join(dir, "segments", "deadbeef", "stream_0_005.m4s")
	assert.Equal(t, expect, c.Path(k, true))
}

func TestSegmentCache_IsReady_FalseWhenMissing(t *testing.T) {
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "nope", VariantIdx: 0, SegmentIdx: 0}
	assert.False(t, c.IsReady(k, true))
}

func TestSegmentCache_IsReady_TrueWhenFileExists(t *testing.T) {
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	path := c.Path(k, true)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("seg"), 0o644))

	assert.True(t, c.IsReady(k, true))
}

func TestSegmentCache_IsReady_FalseWhenTmpSiblingExists(t *testing.T) {
	// Defence in depth: even if the final name exists, a `.tmp` sibling
	// signals an in-progress write (race window between rename and our stat).
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	path := c.Path(k, true)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("seg"), 0o644))
	require.NoError(t, os.WriteFile(path+".tmp", []byte("partial"), 0o644))

	assert.False(t, c.IsReady(k, true))
}

func TestSegmentCache_Wait_ReturnsImmediatelyWhenReady(t *testing.T) {
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Path(k, true)), 0o755))
	require.NoError(t, os.WriteFile(c.Path(k, true), []byte("seg"), 0o644))

	start := time.Now()
	err := c.Wait(context.Background(), k, true, time.Second)
	require.NoError(t, err)
	// Sanity: shouldn't have waited any meaningful time.
	assert.Less(t, time.Since(start), 50*time.Millisecond)
}

func TestSegmentCache_Wait_TimesOutWhenMissing(t *testing.T) {
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}

	err := c.Wait(context.Background(), k, true, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestSegmentCache_Wait_RespectsContextCancel(t *testing.T) {
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := c.Wait(ctx, k, true, time.Second)
		require.ErrorIs(t, err, context.Canceled)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	wg.Wait()
}

func TestSegmentCache_Wait_AbortsOnCleanupHash(t *testing.T) {
	// RegisterSession + CleanupHash combo: a Wait blocked on a session that
	// gets torn down must return promptly, not stall until timeout.
	c, _ := newTestCache(t)
	hash := "abcd"
	k := SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 0}
	teardown := c.RegisterSession(hash)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := c.Wait(context.Background(), k, true, 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "torn down")
	}()
	time.Sleep(30 * time.Millisecond)
	teardown()
	wg.Wait()
}

func TestSegmentCache_Wait_DetectsSegmentAfterDelay(t *testing.T) {
	// Simulates ffmpeg producing a segment a bit after the Wait starts: must
	// pick it up via the poll loop and return success, not time out.
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Path(k, true)), 0o755))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := c.Wait(context.Background(), k, true, 2*time.Second)
		require.NoError(t, err)
	}()

	time.Sleep(80 * time.Millisecond)
	require.NoError(t, os.WriteFile(c.Path(k, true), []byte("seg"), 0o644))
	wg.Wait()
}

func TestSegmentCache_CleanupHash_RemovesFiles(t *testing.T) {
	c, dir := newTestCache(t)
	hash := "abcd"
	k := SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 0}
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Path(k, true)), 0o755))
	require.NoError(t, os.WriteFile(c.Path(k, true), []byte("seg"), 0o644))

	require.NoError(t, c.CleanupHash(hash))

	_, err := os.Stat(filepath.Join(dir, "segments", hash))
	assert.True(t, os.IsNotExist(err), "hash dir should be gone")
}

func TestSegmentCache_CleanupHash_NoopWhenAbsent(t *testing.T) {
	c, _ := newTestCache(t)
	require.NoError(t, c.CleanupHash("never-existed"))
}

func TestSegmentCache_CleanupHash_RejectsEmpty(t *testing.T) {
	c, _ := newTestCache(t)
	require.Error(t, c.CleanupHash(""))
}

// ============================================================================
// SegmentCache.ListReady
// ============================================================================

func TestSegmentCache_ListReady_Empty(t *testing.T) {
	c, _ := newTestCache(t)
	idxs, err := c.ListReady("never-existed", 0, true)
	require.NoError(t, err)
	assert.Nil(t, idxs)
}

func TestSegmentCache_ListReady_OrdersAndFilters(t *testing.T) {
	c, _ := newTestCache(t)
	hash := "abcd"
	// Lay down a mix: variant 0 segments 5, 0, 2, an init segment, and a
	// variant-1 segment that ListReady(0,...) must filter out.
	files := []string{
		"stream_0_005.m4s",
		"stream_0_000.m4s",
		"stream_0_002.m4s",
		"stream_0_init.mp4",
		"stream_1_000.m4s",
	}
	dir, err := c.DirFor(hash)
	require.NoError(t, err)
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644))
	}

	idxs, err := c.ListReady(hash, 0, true)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 2, 5}, idxs)
}

// ============================================================================
// helpers
// ============================================================================

func newTestCache(t *testing.T) (*SegmentCache, string) {
	t.Helper()
	dir := t.TempDir()
	c, err := NewSegmentCache(dir)
	require.NoError(t, err)
	return c, dir
}

func minimalFingerprint(t *testing.T) DecisionFingerprint {
	t.Helper()
	return DecisionFingerprint{
		MediaID:             "media-abc",
		SourcePath:          "/media/test.mkv",
		VideoCodec:          "h264",
		AudioCodec:          "aac",
		AudioTrack:          0,
		SubtitleBurnInTrack: -1,
		HDRTonemap:          false,
		AudioNormalize:      false,
		MaxAudioChannels:    2,
		ForceSidecarSubs:    false,
		UseFMP4:             true,
		Profiles: []ProfileFingerprint{
			{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
		},
	}
}
