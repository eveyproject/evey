package mkvgo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A per-track retimer builds one shift entry per LATE audio track, keyed on the
// real Matroska track number (zipped from the mkvgo Tracks order against the
// ffprobe stream order), and leaves aligned tracks alone. Proven on a real
// two-audio-track file whose tracks disagree on the delay - the exact case the
// uniform ffmpeg remux refuses. Opt-in (needs ffmpeg/ffprobe):
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestAudioRetimer -v
func TestAudioRetimerIntegration_PerTrackShiftMap(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	ctx := t.Context()
	dir := t.TempDir()
	// Build a file: video at 0, audio track A late 0.90s, audio track B aligned.
	src := filepath.Join(dir, "src.mkv")
	mux := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=192x108:rate=24",
		"-itsoffset", "0.900", "-f", "lavfi", "-i", "sine=frequency=300:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=500:sample_rate=48000",
		"-t", "30",
		"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "ac3", "-f", "matroska", src)
	if out, err := mux.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}
	// mkvgo refuses unknown-size (piped) segments, but ffmpeg to a real path
	// seals the size, so RetimeTracksReplace applies directly.

	var gotShift map[uint64]int64
	r := &AudioRetimer{
		ffprobe: "ffprobe",
		retime: func(ctx context.Context, path string, shift map[uint64]int64, opts ...extmatroska.Options) error {
			gotShift = shift
			return extmatroska.RetimeTracksReplace(ctx, path, shift, opts...)
		},
	}

	bak, skipped, err := r.RemuxAudioDelay(ctx, src)
	require.NoError(t, err)
	require.False(t, skipped)

	// Exactly one track shifted (the late one), by ~-0.9s in ns; the aligned
	// track carries no entry.
	require.Len(t, gotShift, 1, "only the late track is shifted: %v", gotShift)
	for _, ns := range gotShift {
		assert.InDelta(t, -900_000_000, ns, 60_000_000, "shifted back by ~its own delay")
	}
	// Backup adopted Evey's convention.
	assert.Equal(t, src+".mkvgobak", bak)
	_, statErr := os.Stat(bak)
	require.NoError(t, statErr, "the .mkvgobak backup exists")

	// Proof: after the retime, BOTH audio tracks start aligned with the video.
	for _, sel := range []string{"a:0", "a:1"} {
		out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
			"-select_streams", sel, "-show_entries", "stream=start_time",
			"-of", "csv=p=0", src).Output()
		require.NoError(t, err)
		var start float64
		if v, ok := parseSeconds(trimLine(string(out))); ok {
			start = v
		}
		assert.InDelta(t, 0, start, retimeDelayFloorSec, "%s aligned after retime", sel)
	}
}

func trimLine(s string) string {
	for i, c := range s {
		if c == '\n' || c == '\r' {
			return s[:i]
		}
	}
	return s
}
