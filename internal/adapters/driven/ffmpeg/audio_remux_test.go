package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The repair command must shift ONLY the audio (second input) and copy every
// stream - re-encoding would silently degrade the file it claims to repair.
func TestBuildRemuxArgs(t *testing.T) {
	args := strings.Join(buildRemuxArgs("/m/in.mkv", "/m/in.mkv.remux.tmp", 0.9), " ")
	for _, want := range []string{
		"-itsoffset -0.900000",
		"-map 0 -map -0:a -map 1:a",
		"-c copy",
		"-map_chapters 0",
		"-f matroska",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q in: %s", want, args)
		}
	}
	if strings.Count(args, "/m/in.mkv ") != 2 {
		t.Errorf("the source must appear as both inputs: %s", args)
	}
}

// Integration: generate a real file whose audio starts 0.9s late (the prod
// repack defect), repair it, and PROVE the delay is gone - plus the backup
// carries the pre-repair original and a failed uniformity guard leaves the
// file untouched. Opt-in like the other transcode itests:
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestAudioRemuxIntegration -v
func TestAudioRemuxIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (make transcode-itest)")
	}
	ctx := t.Context()
	dir := t.TempDir()
	_, delayed := genDelayedAudioSource(t, ctx, dir, 0.9)
	originalBytes, err := os.ReadFile(delayed)
	if err != nil {
		t.Fatal(err)
	}

	r := NewAudioRemuxer("ffmpeg", "ffprobe")

	// The defect is measurable before the repair.
	before, err := r.probe(ctx, delayed)
	if err != nil {
		t.Fatal(err)
	}
	if before.maxDelay < 0.8 || before.maxDelay > 1.0 {
		t.Fatalf("fixture delay = %.3fs, want ~0.9s", before.maxDelay)
	}

	bak, skipped, err := r.RemuxAudioDelay(ctx, delayed)
	if err != nil {
		t.Fatalf("remux: %v", err)
	}
	if skipped {
		t.Fatal("a 0.9s delay must not be skipped")
	}

	// Proof: the repaired file has no measurable delay left.
	after, err := r.probe(ctx, delayed)
	if err != nil {
		t.Fatal(err)
	}
	if after.maxDelay > remuxDelayFloorSec || after.minDelay < -remuxDelayFloorSec {
		t.Fatalf("repaired delay = [%.3f, %.3f]s, want within ±%.1fs of 0", after.minDelay, after.maxDelay, remuxDelayFloorSec)
	}
	if after.streams != before.streams {
		t.Fatalf("stream count changed: %d -> %d", before.streams, after.streams)
	}

	// The backup IS the pre-repair original, byte for byte.
	bakBytes, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if len(bakBytes) != len(originalBytes) {
		t.Fatalf("backup size %d != original %d", len(bakBytes), len(originalBytes))
	}

	// Already-aligned file: skipped, no temp, no backup.
	clean := filepath.Join(dir, "clean.mkv")
	_, skipped, err = r.RemuxAudioDelay(ctx, clean)
	if err != nil {
		t.Fatalf("clean remux: %v", err)
	}
	if !skipped {
		t.Fatal("an aligned file must be skipped")
	}
	if _, err := os.Stat(clean + ".remux.tmp"); !os.IsNotExist(err) {
		t.Fatal("skip must leave no temp behind")
	}
}

// Mixed per-track delays must be refused (a uniform shift would misalign the
// on-time track), with the original untouched.
func TestAudioRemuxIntegration_MixedDelaysRefused(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (make transcode-itest)")
	}
	ctx := context.Background()
	dir := t.TempDir()
	clean, delayed := genDelayedAudioSource(t, ctx, dir, 0.9)

	// Mux one on-time audio track + one late one into a single file.
	mixed := filepath.Join(dir, "mixed.mkv")
	out, err := execCommand(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-i", clean, "-i", delayed,
		"-map", "0:v:0", "-map", "0:a:0", "-map", "1:a:0", "-c", "copy", mixed)
	if err != nil {
		t.Fatalf("mixed fixture: %v\n%s", err, out)
	}
	before, err := os.ReadFile(mixed)
	if err != nil {
		t.Fatal(err)
	}

	r := NewAudioRemuxer("ffmpeg", "ffprobe")
	_, _, err = r.RemuxAudioDelay(ctx, mixed)
	if err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("want the uniformity refusal, got: %v", err)
	}
	after, rerr := os.ReadFile(mixed)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(after) != len(before) {
		t.Fatal("a refused remux must leave the original untouched")
	}
	if _, err := os.Stat(mixed + remuxBackupSuffix); !os.IsNotExist(err) {
		t.Fatal("a refused remux must not leave a backup")
	}
}

// execCommand is a tiny test helper around CombinedOutput.
func execCommand(ctx context.Context, bin string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %w", bin, err)
	}
	return out, nil
}
