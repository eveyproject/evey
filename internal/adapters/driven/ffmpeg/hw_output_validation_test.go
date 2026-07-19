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
