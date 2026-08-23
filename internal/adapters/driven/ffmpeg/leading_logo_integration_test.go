package ffmpeg

// Leading-logo INTEGRATION test: the real IntroDetector + the real ffmpeg
// binary against a synthetic film whose head carries a distributor-logo block
// (black, card, black, card, black) followed by content.
//
// It exists because the unit tests for parseLeadingLogoBlock feed it
// hand-written blackdetect output: they prove the SELECTION rule, and nothing
// at all about whether blackdetect actually emits that shape on a real file at
// the thresholds the detector asks for (d=0.5:pix_th=0.10). That gap is
// exactly where the ground truth lives - the labels say the block ends between
// 27s and 172s, and no synthetic string can confirm the decode agrees.
//
// OPT-IN, same contract as the copy-pipeline itest (it needs the BUNDLED
// ffmpeg, not whatever a distro ships):
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestLeadingLogoBlockIntegration -v

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLeadingLogoBlockIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (needs the bundled ffmpeg)")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic-film.mkv")

	// 3s black, 6s card, 2s black, 6s card, 3s black, then 220s of moving
	// content. The logo block therefore ends at 20s and the file runs 240s, so
	// it clears the 12s floor and stays under the 10%-of-runtime cap (24s).
	const filter = "color=c=black:s=320x180:d=3[a];" +
		"color=c=white:s=320x180:d=6[b];" +
		"color=c=black:s=320x180:d=2[c];" +
		"color=c=gray:s=320x180:d=6[d];" +
		"color=c=black:s=320x180:d=3[e];" +
		"testsrc=s=320x180:d=220[f];" +
		"[a][b][c][d][e][f]concat=n=6:v=1:a=0[v]"
	build := exec.CommandContext(context.Background(), "ffmpeg", "-nostdin", "-y",
		"-filter_complex", filter, "-map", "[v]",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-r", "10", path)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build synthetic film: %v\n%s", err, out)
	}

	d := NewIntroDetector("ffmpeg", "ffprobe")
	startMs, endMs, found, err := d.DetectLeadingLogoBlock(context.Background(), path, 240)
	if err != nil {
		t.Fatalf("DetectLeadingLogoBlock: %v", err)
	}
	if !found {
		t.Fatal("real blackdetect output produced no leading block on a file that opens on 3s of black")
	}
	if startMs != 0 {
		t.Fatalf("the block always starts at 0 by construction, got %d ms", startMs)
	}
	// The last black region of the cluster ends at 20s. Allow a second of slack
	// for the encoder's own fade at the joins.
	if endMs < 18_000 || endMs > 22_000 {
		t.Fatalf("block end should land on the 20s mark, got %d ms", endMs)
	}

	// Control: a film that opens straight on picture must be refused, or the
	// pass would stamp an intro on every movie.
	noBlack := filepath.Join(dir, "no-logo-block.mkv")
	build2 := exec.CommandContext(context.Background(), "ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=s=320x180:d=240:r=10",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", noBlack)
	if out, err := build2.CombinedOutput(); err != nil {
		t.Fatalf("build control film: %v\n%s", err, out)
	}
	_, _, found2, ctrlErr := d.DetectLeadingLogoBlock(context.Background(), noBlack, 240)
	if ctrlErr != nil {
		t.Fatalf("DetectLeadingLogoBlock (control): %v", ctrlErr)
	}
	if found2 {
		t.Fatal("a film that opens on picture must yield no logo block")
	}
}
