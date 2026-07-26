package mkvgo

// Audio tail-gap probe INTEGRATION test on real containers, reproducing the
// prod 2026-07-26 defect shape ("Scary Movie"): 30 s of video, 18 s of audio.
// Covers BOTH stages: the DURATION-tag head fast path (ffmpeg's muxer writes
// the tags) and the Cue-seek tail walk (forced by ignoring the tags), plus a
// healthy control file.
//
// OPT-IN like the other mkvgo itests: run via `make transcode-itest` or
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestAudioTailProbeIntegration -v

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
)

func tracksOf(meta *extmkv.Container, kind extmkv.TrackType) []extmkv.Track {
	var out []extmkv.Track
	for _, tr := range meta.Tracks {
		if tr.Type == kind {
			out = append(out, tr)
		}
	}
	return out
}

func genTailMKV(t *testing.T, ctx context.Context, dir, name string, audioSec int) string {
	t.Helper()
	src := filepath.Join(dir, name)
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-t", "30", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-t", "18", "-i", "anoisesrc=color=pink:seed=42:sample_rate=48000",
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-ac", "2",
		src)
	if audioSec >= 30 {
		gen = exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
			"-f", "lavfi", "-t", "30", "-i", "testsrc2=size=320x180:rate=24",
			"-f", "lavfi", "-t", "30", "-i", "anoisesrc=color=pink:seed=7:sample_rate=48000",
			"-map", "0:v:0", "-map", "1:a:0",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
			"-c:a", "ac3", "-ac", "2",
			src)
	}
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("%s generation: %v\n%s", name, err, out)
	}
	return src
}

func TestAudioTailProbeIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs ffmpeg to build the fixtures)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	short := genTailMKV(t, ctx, dir, "short_audio.mkv", 18)
	healthy := genTailMKV(t, ctx, dir, "full_audio.mkv", 30)
	p := NewAudioTailProber()

	// Stage 1 (tags): the defect must measure ~12 s.
	gap, err := p.ProbeAudioTailGapSec(ctx, short)
	if err != nil {
		t.Fatalf("ProbeAudioTailGapSec: %v", err)
	}
	if gap < 11.0 || gap > 13.0 {
		t.Fatalf("gap = %.3f s, want ~12 s (30 s container, 18 s audio)", gap)
	}

	// Stage 2 forced: ignore the tags so the Cue-seek cluster walk must
	// measure the same truth from the real bytes.
	meta, err := reader.OpenMeta(ctx, short, reader.WithTags(), reader.WithCues())
	if err != nil {
		t.Fatal(err)
	}
	gap2, err := probeAudioTailGapFromClusters(ctx, short, meta,
		tracksOf(meta, extmkv.AudioTrack), tracksOf(meta, extmkv.VideoTrack), map[uint64]float64{})
	if err != nil {
		t.Fatalf("cluster walk: %v", err)
	}
	if gap2 < 11.0 || gap2 > 13.0 {
		t.Fatalf("cluster-walk gap = %.3f s, want ~12 s", gap2)
	}

	// A healthy file must stay near 0 on both stages.
	hgap, err := p.ProbeAudioTailGapSec(ctx, healthy)
	if err != nil {
		t.Fatal(err)
	}
	if hgap > 1.5 {
		t.Fatalf("healthy tag gap = %.3f s, want ~0", hgap)
	}
	hmeta, err := reader.OpenMeta(ctx, healthy, reader.WithTags(), reader.WithCues())
	if err != nil {
		t.Fatal(err)
	}
	hgap2, err := probeAudioTailGapFromClusters(ctx, healthy, hmeta,
		tracksOf(hmeta, extmkv.AudioTrack), tracksOf(hmeta, extmkv.VideoTrack), map[uint64]float64{})
	if err != nil {
		t.Fatal(err)
	}
	if hgap2 > 1.5 {
		t.Fatalf("healthy cluster-walk gap = %.3f s, want ~0", hgap2)
	}
}
