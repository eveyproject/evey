package mkvgo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A complete MKV with surplus bytes appended PAST its declared Segment end is
// trailing junk, not damage: mkvgo v0.25 raises the `trailing-junk` finding
// (and no longer the truncated verdict) for bytes lying entirely beyond the
// Segment, but those bytes still inflate the salvage walk's BytesSkipped. The
// adapter must surface TrailingJunk() true, keep Truncated false, and NOT relay
// the surplus as DamagedBytes - otherwise a whole, playable file is shown as
// corrupt and the operator is told to re-download or resync it for nothing.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestDiagnoseTrailingJunk -v
func TestDiagnoseTrailingJunkIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	ctx := t.Context()
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.mkv")
	// A SEEKABLE output (ffmpeg's own file argument, not a pipe): the muxer
	// seeks back to seal the real Segment size and writes a Cues element. That
	// known declared end is what makes "declared < actual" - trailing junk -
	// detectable; a piped, unknown-size Segment could never be.
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=300", "-t", "6",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48",
		"-c:a", "aac", clean)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}

	a := New()

	// Baseline: the pristine file is not flagged for any tail defect.
	base, err := a.Diagnose(ctx, clean)
	require.NoError(t, err)
	require.False(t, base.TrailingJunk(), "a clean file must not report trailing junk")
	require.False(t, base.Truncated, "a clean file must not report truncation")
	require.Zero(t, base.DamagedBytes, "a clean file has no damaged bytes")

	// Append 8 KiB of junk PAST the declared Segment end.
	junky := filepath.Join(dir, "junky.mkv")
	data, err := os.ReadFile(clean)
	require.NoError(t, err)
	junk := make([]byte, 8192)
	for i := range junk {
		junk[i] = byte(i*31 + 7)
	}
	require.NoError(t, os.WriteFile(junky, append(append([]byte{}, data...), junk...), 0o644))

	d, err := a.Diagnose(ctx, junky)
	require.NoError(t, err)
	assert.True(t, d.TrailingJunk(), "surplus bytes past the Segment end must raise trailing-junk")
	assert.False(t, d.Truncated, "trailing junk is complete media, never a truncation")
	assert.False(t, d.Damaged(), "trailing junk is not mid-Segment damage")
	assert.Zero(t, d.DamagedBytes, "the surplus tail must NOT be relayed as damaged bytes")
}
