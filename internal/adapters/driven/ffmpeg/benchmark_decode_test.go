package ffmpeg

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireFFmpegBin skips when the binaries are absent. Declared here: the
// same-named helper lives in the external ffmpeg_test package, and these probes
// are unexported.
func requireFFmpegBin(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found, skipping integration test", bin)
		}
	}
}

// writeClip renders a tiny real clip, so the probes run against real bitstreams.
// The muxer is forced: ffmpeg infers it from the extension, so H.264 written to
// a .hevc name - the production trap - would be refused before it can be probed.
func writeClip(t *testing.T, dst, encoder, muxer string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=24",
		"-c:v", encoder, "-preset", "ultrafast", "-an", "-f", muxer, dst)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg: %s", tailString(string(out), 300))
}

func decodeTestRunner(t *testing.T, samplePath string) *BenchmarkRunner {
	t.Helper()
	return NewBenchmarkRunner(BenchmarkOpts{
		SamplePath:   samplePath,
		DurationSec:  1,
		BenchSamples: 1,
	}, slog.New(slog.DiscardHandler))
}

// The guard that would have caught two months of mis-calibration.
func TestProbeSourceClass_ReportsTheRealCodecNotTheFilename(t *testing.T) {
	requireFFmpegBin(t)
	dir := t.TempDir()
	// Deliberately the production trap: H.264 bytes behind a .hevc name.
	lying := filepath.Join(dir, "sample.hevc")
	writeClip(t, lying, "libx264", "h264")

	got, ok := decodeTestRunner(t, lying).probeSourceClass(context.Background(), lying)

	require.True(t, ok)
	assert.Equal(t, "h264", got.Codec, "the bytes decide, not the extension")
	assert.Equal(t, 240, got.Height)
}

func TestProbeSourceClass_UnreadableFile_ReportsNotOK(t *testing.T) {
	requireFFmpegBin(t)
	missing := filepath.Join(t.TempDir(), "nope.hevc")
	_, ok := decodeTestRunner(t, missing).probeSourceClass(context.Background(), missing)
	assert.False(t, ok, "an unprobeable sample must yield no evidence, not a zero-value guess")
}

// Decode throughput is the denominator the cost model divides by.
func TestBenchDecodeFPS_MeasuresPositiveThroughput(t *testing.T) {
	requireFFmpegBin(t)
	clip := filepath.Join(t.TempDir(), "clip.mkv")
	writeClip(t, clip, "libx264", "matroska")

	fps, ok := decodeTestRunner(t, clip).benchDecodeFPS(context.Background(), clip)

	require.True(t, ok)
	assert.Positive(t, fps)
}

func TestBenchDecodeFPS_MissingFile_ReportsNotMeasured(t *testing.T) {
	requireFFmpegBin(t)
	missing := filepath.Join(t.TempDir(), "nope.mkv")
	_, ok := decodeTestRunner(t, missing).benchDecodeFPS(context.Background(), missing)
	assert.False(t, ok)
}

// decodeEvidence reports the source it actually measured against, and covers
// every codec the cost model prices.
func TestDecodeEvidence_ReportsProbedSourceAndBothCodecs(t *testing.T) {
	requireFFmpegBin(t)
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.hevc")
	writeClip(t, sample, "libx264", "h264") // an H.264 sample, as production had

	benchSource, samples := decodeTestRunner(t, sample).decodeEvidence(context.Background(), sample, false)

	assert.Equal(t, "h264", benchSource.Codec, "the recorded reference is what was really decoded")
	assert.Equal(t, 240, benchSource.Height)

	byCodec := map[string]float64{}
	for _, s := range samples {
		byCodec[s.Codec] = s.FPS
		assert.Positive(t, s.FPS, "codec %s", s.Codec)
		assert.Equal(t, benchSource.Height, s.Height, "codec %s measured at the sample's height", s.Codec)
	}
	assert.Contains(t, byCodec, "h264")
	assert.Contains(t, byCodec, "hevc", "the codec the sample is NOT must get a companion clip")

	// The companion is cached next to the sample so the next boot reuses it.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var companions []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "sample-decode-") {
			companions = append(companions, e.Name())
		}
	}
	assert.NotEmpty(t, companions, "the companion clip must be cached, not rebuilt every boot")
}

// No file to decode on the synthetic fallback, and a mire's decode cost is not
// a real source's: no evidence beats misleading evidence.
func TestDecodeEvidence_SyntheticFallback_YieldsNothing(t *testing.T) {
	t.Parallel()
	benchSource, samples := decodeTestRunner(t, "").decodeEvidence(context.Background(), "", true)
	assert.False(t, benchSource.Known())
	assert.Empty(t, samples)
}

// A cached sample we generated but whose codec is wrong must be thrown away, not
// believed because the path exists. Exactly the production trap.
func TestCachedSampleIsSound_WrongCodecOurs_Rejected(t *testing.T) {
	requireFFmpegBin(t)
	lying := filepath.Join(t.TempDir(), "sample.hevc")
	writeClip(t, lying, "libx264", "h264")

	r := decodeTestRunner(t, lying)
	assert.False(t, r.cachedSampleIsSound(context.Background(), "SDR", benchSampleSource{}, lying))
}

// An operator's SHA-pinned reference clip is deliberate content: reported, kept.
func TestCachedSampleIsSound_WrongCodecPinnedByOperator_Kept(t *testing.T) {
	requireFFmpegBin(t)
	pinned := filepath.Join(t.TempDir(), "sample.hevc")
	writeClip(t, pinned, "libx264", "h264")

	r := decodeTestRunner(t, pinned)
	src := benchSampleSource{url: "https://example.invalid/clip.mkv", sha256: "deadbeef"}
	assert.True(t, r.cachedSampleIsSound(context.Background(), "SDR", src, pinned))
}

func TestCachedSampleIsSound_RightCodec_Kept(t *testing.T) {
	requireFFmpegBin(t)
	good := filepath.Join(t.TempDir(), "sample.hevc")
	writeClip(t, good, "libx265", "hevc")

	r := decodeTestRunner(t, good)
	assert.True(t, r.cachedSampleIsSound(context.Background(), "SDR", benchSampleSource{}, good))
}

// resolveBenchSample must actually act on the verdict: a wrong-codec clip we own
// is removed so the cascade regenerates it.
func TestResolveBenchSample_WrongCodecCached_RegeneratesInsteadOfTrusting(t *testing.T) {
	requireFFmpegBin(t)
	dir := t.TempDir()
	cached := filepath.Join(dir, "sample.hevc")
	writeClip(t, cached, "libx264", "h264")

	r := decodeTestRunner(t, cached)
	regenerated := false
	path, fallback := r.resolveBenchSample(context.Background(), "SDR", benchSampleSource{}, cached,
		func(dst string) error {
			regenerated = true
			writeClip(t, dst, "libx265", "hevc")
			return nil
		})

	require.False(t, fallback)
	assert.True(t, regenerated, "the wrong-codec clip must be regenerated, not trusted")
	probed, ok := r.probeSourceClass(context.Background(), path)
	require.True(t, ok)
	assert.Equal(t, "hevc", probed.Codec)
}

// A sound cached clip is reused as-is: revalidation must not cost a regeneration
// on every boot.
func TestResolveBenchSample_SoundCached_ReusedWithoutRegenerating(t *testing.T) {
	requireFFmpegBin(t)
	cached := filepath.Join(t.TempDir(), "sample.hevc")
	writeClip(t, cached, "libx265", "hevc")

	r := decodeTestRunner(t, cached)
	path, fallback := r.resolveBenchSample(context.Background(), "SDR", benchSampleSource{}, cached,
		func(string) error {
			t.Fatal("a sound cached sample must never be regenerated")
			return nil
		})

	assert.False(t, fallback)
	assert.Equal(t, cached, path)
}

// Prod, 2026-09-09: sample-hdr10.hevc was 0 bytes. The probe fails on an empty
// file, and "no verdict is not a guilty verdict" then kept it - so the HDR cap
// was benched against nothing. Emptiness needs no probe to be condemned.
func TestCachedSampleIsSound_EmptyFile_Rejected(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "sample-hdr10.hevc")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))

	r := decodeTestRunner(t, empty)
	assert.False(t, r.cachedSampleIsSound(context.Background(), "HDR", benchSampleSource{}, empty),
		"a 0-byte cached sample must be regenerated, not calibrated against")
}

// An empty file the OPERATOR pinned is still empty: the "keep operator content"
// exemption covers a deliberate wrong CODEC, not zero bytes.
func TestCachedSampleIsSound_EmptyFilePinnedByOperator_StillRejected(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "sample-hdr10.hevc")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))

	r := decodeTestRunner(t, empty)
	src := benchSampleSource{url: "https://example.invalid/clip.hevc"}
	assert.False(t, r.cachedSampleIsSound(context.Background(), "HDR", src, empty))
}

// The rate every bench threshold is built on is the SAMPLE's, probed once:
// an operator clip at 60p used to be measured at its real cost and recorded
// as a 24p source, so a route that could not hold its 60 fps passed.
func TestBenchmark_SampleRateFollowsTheClip(t *testing.T) {
	requireFFmpegBin(t)
	dir := t.TempDir()
	clip60 := filepath.Join(dir, "sixty.mkv")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=60",
		"-c:v", "libx264", "-preset", "ultrafast", "-an", "-f", "matroska", clip60)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg: %s", tailString(string(out), 300))
	clip24 := filepath.Join(dir, "twentyfour.mkv")
	writeClip(t, clip24, "libx264", "matroska")

	r := decodeTestRunner(t, clip24)
	ctx := context.Background()
	assert.InDelta(t, 60, r.sampleRate(ctx, clip60, false), 0.01)
	assert.InDelta(t, 24, r.sampleRate(ctx, clip24, false), 0.01)
	assert.InDelta(t, 24, r.sampleRate(ctx, "", true), 0.001, "the lavfi fallback is 24 by construction")
	assert.InDelta(t, 24, r.sampleRate(ctx, filepath.Join(dir, "missing.mkv"), false), 0.001, "an unprobeable file falls back to 24, never to 0")
	assert.InDelta(t, 60, r.sampleRate(ctx, clip60, false), 0.01, "cached: same answer on the second ask")
}
