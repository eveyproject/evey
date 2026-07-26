package ffmpeg

// Audio tail-gap probe INTEGRATION test against the real bundled ffprobe, on
// the same defect shape as prod 2026-07-26 ("Scary Movie"): 30 s of video,
// 18 s of audio. Covers BOTH stages: the head-only DURATION-tag fast path and
// the tail packet-read fallback (forced by bypassing the tag stage).
//
// OPT-IN like the other transcode itests: run via `make transcode-itest` or
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestAudioTailProbeIntegration -v

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestAudioTailProbeIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffprobe)")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not in PATH")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	src := genShortAudioSource(t, ctx, dir)
	p := NewProber("ffprobe")

	// Stage 1 (whatever stage answers): the defect must measure ~12 s.
	gap, err := p.ProbeAudioTailGapSec(ctx, src)
	if err != nil {
		t.Fatalf("ProbeAudioTailGapSec: %v", err)
	}
	if gap < 11.0 || gap > 13.0 {
		t.Fatalf("gap = %.3f s, want ~12 s (30 s container, 18 s audio)", gap)
	}

	// Stage 2 forced: pretend no track carried a DURATION tag so the packet
	// fallback must measure the same truth from the real container bytes.
	head, err := runAudioTailHeadProbe(ctx, "ffprobe", src)
	if err != nil {
		t.Fatal(err)
	}
	dur, audio, err := parseAudioTailHead(head)
	if err != nil {
		t.Fatal(err)
	}
	for i := range audio {
		audio[i].EndSec = -1
	}
	gap2, err := p.probeAudioTailGapFromPackets(ctx, src, dur, audio)
	if err != nil {
		t.Fatalf("packet fallback: %v", err)
	}
	if gap2 < 11.0 || gap2 > 13.0 {
		t.Fatalf("packet-fallback gap = %.3f s, want ~12 s", gap2)
	}

	// A healthy file (audio to the end) must stay at ~0 on both stages.
	healthy := genDelayedAudioSourceClean(t, ctx, dir)
	hgap, err := p.ProbeAudioTailGapSec(ctx, healthy)
	if err != nil {
		t.Fatal(err)
	}
	if hgap > 1.5 {
		t.Fatalf("healthy file gap = %.3f s, want ~0", hgap)
	}
}

// genDelayedAudioSourceClean writes a 30 s MKV whose audio runs to the end.
func genDelayedAudioSourceClean(t *testing.T, ctx context.Context, dir string) string {
	t.Helper()
	src := dir + "/full_audio.mkv"
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-t", "30", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-t", "30", "-i", "anoisesrc=color=pink:seed=7:sample_rate=48000",
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-ac", "2",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("full-audio source generation: %v\n%s", err, out)
	}
	return src
}
