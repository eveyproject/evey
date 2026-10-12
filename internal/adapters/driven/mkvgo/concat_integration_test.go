package mkvgo

// Gapless concat INTEGRATION test: two real, independently-encoded synthetic
// MKV "episodes" (same codec/audio layout, as PlanConcat requires) packaged
// as ONE continuous HLS session - the end-to-end proof that a real
// EXT-X-DISCONTINUITY concat session serves, not just the unit-level cache
// key / arg-count guards.
//
// OPT-IN (needs a real ffmpeg to synthesize Cues-indexed sources):
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run ConcatIntegration -v
//
// or `make transcode-itest` (runs inside the evey-server container).

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

func genEpisode(t *testing.T, ctx context.Context, dir, name string) string {
	t.Helper()
	src := filepath.Join(dir, name)
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "12",
		"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("episode %s generation failed: %v\n%s", name, err, out)
	}
	return src
}

func TestConcatPackagerIntegration_TwoEpisodesGapless(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()

	e1 := genEpisode(t, ctx, dir, "s01e01.mkv")
	e2 := genEpisode(t, ctx, dir, "s01e02.mkv")

	pkg := NewConcatPackager()
	defer pkg.Close()

	plan, err := pkg.Plan(ctx, []string{e1, e2}, apptranscoding.ConcatOptions{SegmentMs: 6000, URLPrefix: "cc_"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if n := plan.NumParts(); n != 2 {
		t.Fatalf("NumParts = %d, want 2", n)
	}

	master, ct, err := plan.Resource(ctx, "master.m3u8")
	if err != nil {
		t.Fatalf("Resource(master.m3u8): %v", err)
	}
	if !bytes.HasPrefix(master, []byte("#EXTM3U")) {
		t.Errorf("master playlist does not start with #EXTM3U:\n%s", master)
	}
	if !strings.Contains(ct, "mpegurl") {
		t.Errorf("master content-type = %q, want an HLS type", ct)
	}

	videoPlaylist, _, err := plan.Resource(ctx, "playlist.m3u8")
	if err != nil {
		t.Fatalf("Resource(playlist.m3u8): %v", err)
	}
	if !bytes.Contains(videoPlaylist, []byte("EXT-X-DISCONTINUITY")) {
		t.Errorf("concatenated playlist has no EXT-X-DISCONTINUITY (the gapless episode-boundary marker):\n%s", videoPlaylist)
	}

	// Part 0's and part 1's first segments both come through, prefixed by
	// their own p{k}/ path - the actual A/V bytes a player would fetch.
	names := plan.Resources()
	for k, part := range []string{"p0/", "p1/"} {
		found := false
		for _, n := range names {
			if strings.HasPrefix(n, part) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no resource under %s for part %d in Resources(): %v", part, k, names)
		}
	}
	seg, _, err := plan.Resource(ctx, "p0/seg00001.m4s")
	if err != nil {
		t.Fatalf("Resource(p0/seg00001.m4s): %v", err)
	}
	if len(seg) < 8 || !bytes.Equal(seg[4:8], []byte("styp")) {
		t.Errorf("part 0 segment 1 is not a CMAF fragment, got %d bytes", len(seg))
	}
	seg2, _, err := plan.Resource(ctx, "p1/seg00001.m4s")
	if err != nil {
		t.Fatalf("Resource(p1/seg00001.m4s): %v", err)
	}
	if len(seg2) < 8 || !bytes.Equal(seg2[4:8], []byte("styp")) {
		t.Errorf("part 1 segment 1 is not a CMAF fragment, got %d bytes", len(seg2))
	}
}
