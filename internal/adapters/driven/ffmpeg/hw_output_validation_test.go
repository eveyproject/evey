package ffmpeg

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeValidationSample builds a small deterministic H.264 clip. testsrc frames
// are fully reproducible, so a frame-accurate extraction at the same timestamp
// from the sample and from a faithful re-encode of it depicts the same picture.
func makeValidationSample(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	dst := filepath.Join(t.TempDir(), "sample.mp4")
	out, err := exec.Command("ffmpeg", "-hide_banner", "-nostats",
		"-f", "lavfi", "-i", "testsrc=duration=5:size=640x360:rate=24",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-y", dst).CombinedOutput()
	require.NoError(t, err, string(out))
	return dst
}

// A faithful encode of the probe clip must land well under the corruption
// threshold: the decoded output frames depict the same pictures as the
// software-decoded source frames, so their perceptual hashes are a few bits
// apart at most.
func TestHWVisualValidation_PassesFaithfulEncode(t *testing.T) {
	sample := makeValidationSample(t)
	r := NewBenchmarkRunner(BenchmarkOpts{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	verdict, err := r.validateHWVisualOutput(context.Background(), sample, false, "libx264", nil, "")
	require.NoError(t, err)
	assert.True(t, verdict.OK, "a faithful encode must be certified (worst hamming %d)", verdict.WorstHamming)
	assert.Less(t, verdict.WorstHamming, hwCorruptHammingThreshold)
}

// Simulates the silent-corruption failure mode the gate exists for: the
// encoder exits 0 at healthy speed but the pixels are garbage. The corruption
// is injected through the same uploadChain seam benchHardware uses for the
// decoupled path, so the exact production code path is exercised - `negate`
// inverts every plane, the visual equivalent of a broken driver's
// green/purple output.
func TestHWVisualValidation_FlagsCorruptOutput(t *testing.T) {
	sample := makeValidationSample(t)
	r := NewBenchmarkRunner(BenchmarkOpts{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	verdict, err := r.validateHWVisualOutput(context.Background(), sample, false, "libx264", nil, "negate")
	require.NoError(t, err)
	assert.False(t, verdict.OK, "a corrupting pipeline must NOT be certified (worst hamming %d)", verdict.WorstHamming)
	assert.GreaterOrEqual(t, verdict.WorstHamming, hwCorruptHammingThreshold)
}

// The reference frames must come from a SOFTWARE decode of the source: the
// validator never passes the candidate path's input args (-hwaccel ...) to the
// reference extraction, otherwise a decode-side corruption would corrupt both
// sides identically and cancel itself out.
func TestHWVisualValidation_SourceExtractionIgnoresHWInputArgs(t *testing.T) {
	sample := makeValidationSample(t)
	r := NewBenchmarkRunner(BenchmarkOpts{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Bogus input args would make the ENCODE fail loudly if applied, and must
	// never reach the reference extraction. An inconclusive gate (encode
	// error) reports err and no verdict.
	_, err := r.validateHWVisualOutput(context.Background(), sample, false, "libx264",
		[]string{"-hwaccel", "definitely_not_a_real_accel"}, "")
	assert.Error(t, err, "a broken candidate path must surface as inconclusive, not as certified")
}

// The regression that let prod certify hardware on speed alone: the offsets were
// fixed at {1.0, 3.0} while sample.hevc ran 2.88 s, so the 3 s extraction failed
// on EVERY run and every verdict came back inconclusive. Derived offsets always
// land inside the probe output.
func TestHWValidateOffsets_StayInsideTheWindow(t *testing.T) {
	for _, dur := range []float64{0.5, 1.0, 2.88, 4.0, 30.0} {
		offsets := hwValidateOffsets(dur)
		require.NotEmpty(t, offsets, "a positive duration must yield at least one sample point (%.2fs)", dur)
		for _, off := range offsets {
			assert.Greater(t, off, 0.0, "offset 0 is a keyframe on every path, never diagnostic (%.2fs)", dur)
			assert.Less(t, off, dur, "an offset past the end fails the extraction and voids the gate (%.2fs)", dur)
		}
	}
}

// Two points far apart are what separate "corrupt from frame one" from "degrades
// after the first GOP". A window long enough must never collapse to one.
func TestHWValidateOffsets_SpreadsTwoPointsWhenThereIsRoom(t *testing.T) {
	offsets := hwValidateOffsets(4.0)
	require.Len(t, offsets, 2)
	assert.Greater(t, offsets[1]-offsets[0], 0.4)
}

// No duration means the gate cannot conclude, and must say so rather than
// certify: the caller treats an empty offset list as an error, never a pass.
func TestHWValidateOffsets_NoDurationYieldsNoSamplePoints(t *testing.T) {
	assert.Empty(t, hwValidateOffsets(0))
	assert.Empty(t, hwValidateOffsets(-1))
}

// End to end on a clip SHORTER than hwValidateEncodeSeconds - the production
// shape. This used to fail the extraction at 3 s and return an error.
func TestHWVisualValidation_CertifiesAClipShorterThanTheProbeWindow(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	short := filepath.Join(t.TempDir(), "short.mp4")
	out, err := exec.Command("ffmpeg", "-hide_banner", "-nostats",
		"-f", "lavfi", "-i", "testsrc=duration=2.88:size=640x360:rate=24",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-y", short).CombinedOutput()
	require.NoError(t, err, string(out))

	r := NewBenchmarkRunner(BenchmarkOpts{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	verdict, err := r.validateHWVisualOutput(context.Background(), short, false, "libx264", nil, "")
	require.NoError(t, err, "a sample shorter than the probe window must still yield a verdict")
	assert.True(t, verdict.OK, "worst hamming %d", verdict.WorstHamming)
}
