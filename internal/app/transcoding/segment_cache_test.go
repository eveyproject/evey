package transcoding

import (
	"context"
	"encoding/binary"
	"fmt"
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
	first := fp.Hash()
	assert.Equal(t, first, fp.Hash())
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
	// Two decisions that differ on audio track MUST land in separate caches -
	// otherwise the wrong-audio segment would be served to one of them.
	fpA := minimalFingerprint(t)
	fpA.AudioTrack = 0
	fpB := minimalFingerprint(t)
	fpB.AudioTrack = 1
	assert.NotEqual(t, fpA.Hash(), fpB.Hash())
}

func TestDecisionFingerprint_Hash_DistinguishesContainer(t *testing.T) {
	// fMP4 segments are byte-incompatible with TS - must hash to different keys.
	fpA := minimalFingerprint(t)
	fpA.UseFMP4 = false
	fpB := minimalFingerprint(t)
	fpB.UseFMP4 = true
	assert.NotEqual(t, fpA.Hash(), fpB.Hash())
}

// ============================================================================
// AudioFingerprint.Hash
// ============================================================================

func TestAudioFingerprint_Hash_IsStable(t *testing.T) {
	a := AudioFingerprint{MediaID: "m1", SourcePath: "/m.mkv", SourceTrackIdx: 0, AudioCodec: "aac"}
	first := a.Hash()
	assert.Equal(t, first, a.Hash())
}

func TestAudioFingerprint_Hash_DistinguishesTrack(t *testing.T) {
	a1 := AudioFingerprint{MediaID: "m1", SourcePath: "/m.mkv", SourceTrackIdx: 0, AudioCodec: "aac"}
	a2 := AudioFingerprint{MediaID: "m1", SourcePath: "/m.mkv", SourceTrackIdx: 1, AudioCodec: "aac"}
	assert.NotEqual(t, a1.Hash(), a2.Hash(), "different source tracks must NOT share a cache dir")
}

func TestAudioFingerprint_Hash_DistinguishesCodec(t *testing.T) {
	a1 := AudioFingerprint{MediaID: "m1", SourcePath: "/m.mkv", AudioCodec: "aac"}
	a2 := AudioFingerprint{MediaID: "m1", SourcePath: "/m.mkv", AudioCodec: "ac3"}
	assert.NotEqual(t, a1.Hash(), a2.Hash())
}

func TestAudioFingerprint_Hash_NamespaceDisjointFromVideo(t *testing.T) {
	// The two fingerprint types live in disjoint hash spaces - even when
	// every shared field happens to match. Verifies the "audio:" namespace
	// prefix actually does its job.
	dec := DecisionFingerprint{MediaID: "m1", SourcePath: "/m.mkv", AudioCodec: "aac"}
	aud := AudioFingerprint{MediaID: "m1", SourcePath: "/m.mkv", AudioCodec: "aac"}
	assert.NotEqual(t, dec.Hash(), aud.Hash(), "audio and video hash spaces must never collide")
}

// ============================================================================
// SegmentKey.fileName
// ============================================================================

func TestSegmentKey_FileName(t *testing.T) {
	cases := []struct {
		name   string
		k      SegmentKey
		fmp4   bool
		expect string
	}{
		{"ts variant 0 seg 0", SegmentKey{VariantIdx: 0, SegmentIdx: 0}, false, "stream_0_000.ts"},
		{"ts variant 1 seg 42", SegmentKey{VariantIdx: 1, SegmentIdx: 42}, false, "stream_1_042.ts"},
		{"fmp4 variant 2 seg 7", SegmentKey{VariantIdx: 2, SegmentIdx: 7}, true, "stream_2_007.m4s"},
		{"fmp4 init variant 0", SegmentKey{VariantIdx: 0, IsInit: true}, true, "stream_0_init.mp4"},
		{"fmp4 init variant 3", SegmentKey{VariantIdx: 3, IsInit: true}, true, "stream_3_init.mp4"},
		{"ts init still .mp4 - init is always fMP4", SegmentKey{VariantIdx: 0, IsInit: true}, false, "stream_0_init.mp4"},
		// Audio rendition path: separate namespace, always fMP4.
		{"audio track 0 seg 0", SegmentKey{Kind: KindAudio, VariantIdx: 0, SegmentIdx: 0}, true, "audio_0_000.m4s"},
		{"audio track 2 seg 42", SegmentKey{Kind: KindAudio, VariantIdx: 2, SegmentIdx: 42}, true, "audio_2_042.m4s"},
		{"audio init track 1", SegmentKey{Kind: KindAudio, VariantIdx: 1, IsInit: true}, true, "audio_1_init.mp4"},
		{"audio ignores useFMP4=false (always m4s)", SegmentKey{Kind: KindAudio, VariantIdx: 0, SegmentIdx: 3}, false, "audio_0_003.m4s"},
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
	require.NoError(t, os.WriteFile(path, fmp4Seg(), 0o644))

	assert.True(t, c.IsReady(k, true))
}

func TestSegmentCache_IsReady_FalseWhenZeroBytes(t *testing.T) {
	// The fMP4 init segment is written non-atomically (no temp_file rename), so
	// it exists at 0 bytes for a window. A 0-byte file must NOT be reported
	// ready - serving an empty init breaks the player's SourceBuffer init and
	// causes endless buffering. Wait must keep blocking until ffmpeg writes it.
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, IsInit: true}
	path := c.Path(k, true)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte{}, 0o644)) // 0 bytes
	assert.False(t, c.IsReady(k, true), "0-byte init must be not-ready")

	// Once ffmpeg writes real bytes, it becomes ready.
	require.NoError(t, os.WriteFile(path, fmp4Init(), 0o644))
	assert.True(t, c.IsReady(k, true))
}

func TestSegmentCache_IsReady_FalseWhenTmpSiblingExists(t *testing.T) {
	// Defence in depth: even if the final name exists, a `.tmp` sibling
	// signals an in-progress write (race window between rename and our stat).
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	path := c.Path(k, true)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, fmp4Seg(), 0o644))
	require.NoError(t, os.WriteFile(path+".tmp", []byte("partial"), 0o644))

	assert.False(t, c.IsReady(k, true))
}

func TestSegmentCache_IsReady_FalseWhenStypOnly(t *testing.T) {
	// An aborted ffmpeg mux under load can leave a non-empty but malformed fMP4
	// segment at the final name - observed in prod as a 24-byte styp-only `.m4s`
	// (no moof/mdat). It passes the size>0 check but breaks the player's
	// SourceBuffer if served, and silently wedges the ABR fallback. It must be
	// reported not-ready so Wait re-spawns the rafale.
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 7}
	path := c.Path(k, true)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, stypOnlySeg(), 0o644))
	require.NotEmpty(t, stypOnlySeg(), "fixture must be non-empty (passes the size>0 gate)")

	assert.False(t, c.IsReady(k, true), "styp-only segment must be not-ready")

	// A real media segment (styp+moof+mdat) at the same key IS ready.
	require.NoError(t, os.WriteFile(path, fmp4Seg(), 0o644))
	assert.True(t, c.IsReady(k, true))
}

func TestSegmentCache_IsReady_NonFMP4_SkipsContentCheck(t *testing.T) {
	// The fMP4 box-content check must NOT run for MPEG-TS (.ts) segments - they
	// have no ISO-BMFF boxes. A non-empty .ts file is ready on size>0 alone.
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	path := c.Path(k, false)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("\x47\x40\x00\x10raw ts bytes"), 0o644))

	assert.True(t, c.IsReady(k, false), "non-empty TS segment is ready without a box check")
}

func TestSegmentCache_Wait_ReturnsImmediatelyWhenReady(t *testing.T) {
	c, _ := newTestCache(t)
	k := SegmentKey{DecisionHash: "abcd", VariantIdx: 0, SegmentIdx: 0}
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Path(k, true)), 0o755))
	require.NoError(t, os.WriteFile(c.Path(k, true), fmp4Seg(), 0o644))

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
	wg.Go(func() {
		err := c.Wait(ctx, k, true, time.Second)
		assert.ErrorIs(t, err, context.Canceled)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	wg.Wait()
}

// TestSegmentCache_RegisterSession_RefcountedTeardown - the multi-rafale
// orchestrator can RegisterSession twice on the same hash (two clients
// watching the same film at different positions). The release returned
// by RegisterSession must NOT close the shared pending channel until
// EVERY rafale has released - otherwise Wait() callers for the
// still-active rafale receive a spurious "session torn down" abort.
func TestSegmentCache_RegisterSession_RefcountedTeardown(t *testing.T) {
	c, _ := newTestCache(t)
	const hash = "shared-hash"

	release1 := c.RegisterSession(hash)
	release2 := c.RegisterSession(hash)
	// Snapshot the channel BEFORE the first release so we can assert
	// it survives the partial teardown.
	c.mu.RLock()
	ch := c.pending[hash]
	c.mu.RUnlock()
	require.NotNil(t, ch, "RegisterSession must create the pending channel")

	// First release: refcount drops 2→1, channel must stay open so
	// the second rafale's Wait() callers don't trip on it.
	release1()
	select {
	case <-ch:
		t.Fatal("first release closed the shared channel - would break Wait() for the surviving rafale")
	default:
	}

	// Second release: refcount drops 1→0, channel closes.
	release2()
	select {
	case <-ch:
		// expected
	default:
		t.Fatal("last release must close the shared channel")
	}
}

// TestSegmentCache_RegisterSession_DoubleReleaseSafe - both the
// orchestrator's killSession path and the reap goroutine can hold a
// reference to the same release func; if both fire on a normal exit,
// the release MUST be idempotent (no double-decrement of refcount).
func TestSegmentCache_RegisterSession_DoubleReleaseSafe(t *testing.T) {
	c, _ := newTestCache(t)
	release := c.RegisterSession("h")
	release()
	release() // second call must be a no-op
	// Refcount must not have gone negative.
	c.mu.RLock()
	refs := c.pendingRefs["h"]
	c.mu.RUnlock()
	assert.Equal(t, 0, refs, "second release must not decrement past zero")
}

func TestSegmentCache_Wait_AbortsOnCleanupHash(t *testing.T) {
	// RegisterSession + CleanupHash combo: a Wait blocked on a session that
	// gets torn down must return promptly, not stall until timeout.
	c, _ := newTestCache(t)
	hash := "abcd"
	k := SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 0}
	teardown := c.RegisterSession(hash)

	var wg sync.WaitGroup
	wg.Go(func() {
		err := c.Wait(context.Background(), k, true, 5*time.Second)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "torn down")
		}
	})
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
	wg.Go(func() {
		err := c.Wait(context.Background(), k, true, 2*time.Second)
		assert.NoError(t, err)
	})

	time.Sleep(80 * time.Millisecond)
	require.NoError(t, os.WriteFile(c.Path(k, true), fmp4Seg(), 0o644))
	wg.Wait()
}

func TestSegmentCache_CleanupHash_RemovesFiles(t *testing.T) {
	c, dir := newTestCache(t)
	hash := "abcd"
	k := SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 0}
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Path(k, true)), 0o755))
	require.NoError(t, os.WriteFile(c.Path(k, true), fmp4Seg(), 0o644))

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
// SegmentCache.HighestReadyFrom
// ============================================================================

// writeSegs lays down ready fMP4 media segments for variant `variant` at each
// of `idxs`, under the hash directory.
func writeSegs(t *testing.T, c *SegmentCache, hash string, variant int, idxs ...int) {
	t.Helper()
	dir, err := c.DirFor(hash)
	require.NoError(t, err)
	for _, i := range idxs {
		name := fmt.Sprintf("stream_%d_%03d.m4s", variant, i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), fmp4Seg(), 0o644))
	}
}

func TestSegmentCache_HighestReadyFrom_NothingReady(t *testing.T) {
	c, _ := newTestCache(t)
	// from-1 is the "nothing produced yet" sentinel, so a caller can feed the
	// result straight back as the next `from` without special-casing.
	assert.Equal(t, 4, c.HighestReadyFrom("never-existed", 0, true, 5, 100))
}

func TestSegmentCache_HighestReadyFrom_AdvancesOverContiguousRun(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegs(t, c, "abcd", 0, 0, 1, 2, 3)
	assert.Equal(t, 3, c.HighestReadyFrom("abcd", 0, true, 0, 100))
}

func TestSegmentCache_HighestReadyFrom_StopsAtFirstGap(t *testing.T) {
	c, _ := newTestCache(t)
	// 4 exists but 2 does not. The throttle measures how far the client can
	// actually play ahead, and a hole at 2 stalls it there - counting 4 would
	// overstate the buffer and pause ffmpeg on a starved client.
	writeSegs(t, c, "abcd", 0, 0, 1, 3, 4)
	assert.Equal(t, 1, c.HighestReadyFrom("abcd", 0, true, 0, 100))
}

func TestSegmentCache_HighestReadyFrom_ClampsToUntil(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegs(t, c, "abcd", 0, 0, 1, 2, 3, 4, 5)
	// A rafale never produces past endSeg, so the probe must not walk beyond it.
	assert.Equal(t, 2, c.HighestReadyFrom("abcd", 0, true, 0, 2))
}

func TestSegmentCache_HighestReadyFrom_IgnoresOtherVariants(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegs(t, c, "abcd", 1, 0, 1, 2)
	// Variant 0 has produced nothing even though variant 1 is well ahead.
	assert.Equal(t, -1, c.HighestReadyFrom("abcd", 0, true, 0, 100))
	assert.Equal(t, 2, c.HighestReadyFrom("abcd", 1, true, 0, 100))
}

func TestSegmentCache_HighestReadyFrom_RejectsIncompleteSegment(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegs(t, c, "abcd", 0, 0)
	dir, err := c.DirFor("abcd")
	require.NoError(t, err)
	// A styp-only segment is the aborted-mux case IsReady already rejects; the
	// forward probe must inherit that judgement, not just test for existence.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_0_001.m4s"), stypOnlySeg(), 0o644))
	assert.Equal(t, 0, c.HighestReadyFrom("abcd", 0, true, 0, 100))
}

// ============================================================================
// helpers
// ============================================================================

// mp4Box builds one ISO-BMFF box: 4-byte big-endian size, 4-byte type, body.
func mp4Box(typ string, body ...byte) []byte {
	b := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(b[:4], uint32(8+len(body)))
	copy(b[4:8], typ)
	copy(b[8:], body)
	return b
}

// fmp4Seg is a minimal but structurally valid fMP4 media segment
// (styp + moof + mdat) - enough for IsReady's content-box check to accept it.
func fmp4Seg() []byte {
	out := mp4Box("styp", 'm', 's', 'd', 'h')
	out = append(out, mp4Box("moof")...)
	return append(out, mp4Box("mdat", 0, 0, 0, 0)...)
}

// fmp4Init is a minimal valid fMP4 init segment (ftyp + moov).
func fmp4Init() []byte {
	out := mp4Box("ftyp", 'i', 's', 'o', '6')
	return append(out, mp4Box("moov")...)
}

// stypOnlySeg reproduces the malformed 24-byte styp-only segment observed in
// prod under load (an aborted ffmpeg mux): a lone styp box, no moof/mdat.
func stypOnlySeg() []byte {
	return mp4Box("styp", 'm', 's', 'd', 'h', 0, 0, 0, 0, 'm', 's', 'd', 'h', 'm', 's', 'i', 'x')
}

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

func TestDecisionFingerprint_Hash_DistinguishesAudioCopy(t *testing.T) {
	// Copied audio and AAC-re-encoded audio are different bytes for the same
	// media, so the AudioCopy dimension MUST fork the cache. Explicit test
	// because DecisionFingerprint has no reflexive every-field-affects-hash
	// guard (unlike ProfileFingerprint): a silently-ignored field here would
	// alias passthrough and transcoded segments under one hash.
	fpA := minimalFingerprint(t)
	fpA.AudioCopy = false
	fpB := minimalFingerprint(t)
	fpB.AudioCopy = true
	assert.NotEqual(t, fpA.Hash(), fpB.Hash())

	// Conditional emission: AudioCopy=false must keep the PRE-FEATURE hash so
	// turning the feature on does not orphan every existing cache entry whose
	// produced bytes are unchanged.
	assert.Equal(t, fpA.Hash(), minimalFingerprint(t).Hash())
}

func TestDecisionFingerprint_Hash_DistinguishesDeinterlaceAndSAR(t *testing.T) {
	// B3: deint=all vs flag-gated, and anamorphic vs square, produce different
	// bytes ⇒ must fork the cache, but emit conditionally so progressive/square
	// titles keep their pre-feature hash.
	base := minimalFingerprint(t)

	deint := minimalFingerprint(t)
	deint.DeinterlaceAll = true
	assert.NotEqual(t, base.Hash(), deint.Hash(), "deint=all must fork the cache")

	sar := minimalFingerprint(t)
	sar.SourceSAR = "64:45"
	assert.NotEqual(t, base.Hash(), sar.Hash(), "anamorphic SAR must fork the cache")

	// Conditional: defaults (false / "" / "1:1") keep the pre-feature hash.
	assert.Equal(t, base.Hash(), minimalFingerprint(t).Hash())
	square := minimalFingerprint(t)
	square.SourceSAR = "1:1"
	assert.Equal(t, base.Hash(), square.Hash(), "square pixels must NOT fork the cache")

	// Equivalent SAR ratios normalise to the same hash (gcd-reduced).
	sar2 := minimalFingerprint(t)
	sar2.SourceSAR = "128:90" // == 64:45
	assert.Equal(t, sar.Hash(), sar2.Hash(), "equivalent SAR ratios must hash identically")
}

func TestDecisionFingerprint_AudioDecision(t *testing.T) {
	fp := minimalFingerprint(t)
	assert.Equal(t, "transcode", fp.AudioDecision(), "default = historical re-encode")
	fp.AudioCopy = true
	assert.Equal(t, "copy", fp.AudioDecision())
}

// A user audio resync produces different bytes for the same media, so it MUST
// fork the cache hash - conditionally (zero keeps the pre-feature hash so no
// existing cache entry is orphaned).
func TestDecisionFingerprint_Hash_DistinguishesAudioOffset(t *testing.T) {
	base := DecisionFingerprint{MediaID: "m1", SourcePath: "/f.mkv", VideoCodec: "h264", AudioCodec: "aac"}
	shifted := base
	shifted.AudioOffsetMs = 300
	if base.Hash() == shifted.Hash() {
		t.Fatal("a non-zero audio offset must fork the cache hash")
	}
	zero := base
	zero.AudioOffsetMs = 0
	if base.Hash() != zero.Hash() {
		t.Fatal("zero offset must keep the pre-feature hash")
	}
}

// The tone-map implementation is part of an HDR cache key: a VPP segment and
// an OpenCL segment of the same title must never share a hash (prod 2026-09-10).
func TestDecisionFingerprintHash_TonemapImplementationForksHDR(t *testing.T) {
	base := DecisionFingerprint{MediaID: "m", SourcePath: "/f.mkv", HDRTonemap: true}
	sw := base.Hash()
	vpp := base
	vpp.HWTonemap = true
	ocl := vpp
	ocl.HWFeatures = []string{"tonemap_opencl"}
	if sw == vpp.Hash() || vpp.Hash() == ocl.Hash() {
		t.Fatalf("tone-map implementations must fork the cache: sw=%s vpp=%s ocl=%s", sw, vpp.Hash(), ocl.Hash())
	}
	sdr := DecisionFingerprint{MediaID: "m", SourcePath: "/f.mkv"}
	sdrHW := sdr
	sdrHW.HWTonemap = true
	if sdr.Hash() != sdrHW.Hash() {
		t.Fatalf("an SDR title ignores the tone-mapper: %s vs %s", sdr.Hash(), sdrHW.Hash())
	}
}

// The client level cap never lowers a rung's level, it removes the rungs above
// it: its whole effect is the ladder and each rung's level, both hashed per
// rung. Two clients served the same ladder share one encode whatever their cap.
func TestDecisionFingerprint_Hash_RungLevelIsTheIdentity(t *testing.T) {
	base := minimalFingerprint(t)
	base.VideoCodec = "h264"
	base.Profiles = []ProfileFingerprint{{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192, CodecLevel: 40}}

	same := base
	same.Profiles = []ProfileFingerprint{base.Profiles[0]}
	assert.Equal(t, base.Hash(), same.Hash())

	other := base
	other.Profiles = []ProfileFingerprint{base.Profiles[0]}
	other.Profiles[0].CodecLevel = 42
	assert.NotEqual(t, base.Hash(), other.Hash(), "another level is another SPS")

	dropped := base
	dropped.Profiles = nil
	assert.NotEqual(t, base.Hash(), dropped.Hash(), "a rung the cap removed is another ladder")
}

// The ENCODE backend is part of the cache identity: a session on one backend
// must never reuse another's segments/init. A recalibration that flips the
// per-route backend (VAAPI<->QSV) yields a different hash, so the two never
// alias. A software session (no hwenc) keeps its pre-feature hash, and hw and
// software fork from each other.
func TestDecisionFingerprint_Hash_ForksByBackend(t *testing.T) {
	base := DecisionFingerprint{MediaID: "m1", VideoCodec: "h264", AudioTrack: 0,
		Profiles: []ProfileFingerprint{{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192}}}
	sw := base
	vaapi := base
	vaapi.HWAccel = "vaapi"
	qsv := base
	qsv.HWAccel = "qsv"

	if vaapi.Hash() == qsv.Hash() {
		t.Fatal("vaapi and qsv sessions must not share a cache hash")
	}
	if sw.Hash() == vaapi.Hash() || sw.Hash() == qsv.Hash() {
		t.Fatal("a software session must not share a hash with a hardware one")
	}
	// HWDevice / HWFeatures stay out of the hash (same backend = same bytes).
	v2 := vaapi
	v2.HWDevice = "/dev/dri/renderD129"
	v2.HWFeatures = []string{"low_power_h264"}
	if v2.Hash() != vaapi.Hash() {
		t.Fatal("the render node and tuning features must not fork the cache")
	}
	// A software fingerprint's hash is unchanged by the feature (no hwenc term):
	// existing software-cached segments stay addressable.
	swBaseline := base
	if sw.Hash() != swBaseline.Hash() {
		t.Fatal("software hash must be stable across the feature")
	}
}
