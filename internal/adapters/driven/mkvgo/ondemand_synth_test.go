package mkvgo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Matroska with no Cues index used to make PlanHLS refuse (falling the
// player back to a full ffmpeg transcode). With SynthesizeIndex wired the
// packager walks the clusters once and serves it verbatim - the fast-seek
// path for un-indexed and read-only media.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestOnDemandSynthesize -v
func TestOnDemandSynthesizeNoCue(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	ctx := context.Background()
	dir := t.TempDir()
	nocue := filepath.Join(dir, "nocue.mkv")
	// Output to stdout ("-"): ffmpeg cannot seek back to write Cues, so the
	// file has none - exactly the un-indexed source we want to serve.
	fh, err := os.Create(nocue)
	require.NoError(t, err)
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=300",
		"-t", "20", "-c:v", "libx264", "-preset", "ultrafast", "-g", "48",
		"-c:a", "aac", "-f", "matroska", "-")
	gen.Stdout = fh
	if err = gen.Run(); err != nil {
		_ = fh.Close()
		t.Fatalf("fixture: %v", err)
	}
	require.NoError(t, fh.Close())
	// Sanity: the source truly has no seek index.
	has, hErr := New().HasSeekIndex(ctx, nocue)
	require.NoError(t, hErr)
	require.False(t, has, "fixture must be a no-Cue file")

	p := NewOnDemandPackager()
	defer p.Close()
	plan, err := p.Plan(ctx, nocue, apptranscoding.OnDemandOptions{})
	require.NoError(t, err, "SynthesizeIndex must serve a no-Cue Matroska instead of refusing")

	master, ct, err := plan.Resource(ctx, "master.m3u8")
	require.NoError(t, err)
	assert.Contains(t, ct, "mpegurl")
	assert.Contains(t, string(master), "#EXTM3U", "a real master playlist is served")
}
