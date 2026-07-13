package mkvgo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The native prober reports the max audio start delay without ffprobe.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestNativeAudioDelayProber -v
func TestNativeAudioDelayProberIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	ctx := context.Background()
	dir := t.TempDir()
	delayed := filepath.Join(dir, "d.mkv")
	// video at 0, audio track late 0.9s.
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=192x108:rate=24",
		"-itsoffset", "0.9", "-f", "lavfi", "-i", "sine=frequency=300",
		"-t", "20", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "ac3", delayed)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}
	got, err := NewAudioDelayProber().ProbeMaxAudioStartDelaySec(ctx, delayed)
	require.NoError(t, err)
	assert.InDelta(t, 0.9, got, 0.06, "native probe reports the ~0.9s delay")
}

// A non-Matroska container reports 0 without error (the scan never feeds it
// here, but the guard must hold).
func TestNativeAudioDelayProber_NonMatroskaZero(t *testing.T) {
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "x.mp4")
	require.NoError(t, os.WriteFile(mp4, []byte("x"), 0o644))
	got, err := NewAudioDelayProber().ProbeMaxAudioStartDelaySec(context.Background(), mp4)
	require.NoError(t, err)
	assert.Zero(t, got)
}
