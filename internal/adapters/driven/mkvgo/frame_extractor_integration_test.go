package mkvgo_test

// End-to-end snapshot fast path: the real mkvgo keyframe extractor +
// the real ffmpeg packet decoder against an ffmpeg-synthesized MKV, asserting a
// valid JPEG comes out. Proves the two halves (mkvgo seeks/extracts, ffmpeg
// decodes one frame) actually compose on a real file.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run Integration -v

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
)

func TestKeyframeSnapshotIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-t", "20", "-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}

	a := mkvgoadapter.New()
	packet, ext, ptsMs, err := a.ExtractKeyframePacket(ctx, src, 8000)
	if err != nil {
		t.Fatalf("ExtractKeyframePacket: %v", err)
	}
	if len(packet) == 0 {
		t.Fatal("empty keyframe packet")
	}
	if ext != ".h264" {
		t.Errorf("ext = %q, want .h264 for an H.264 source", ext)
	}
	if ptsMs < 0 || ptsMs > 8000 {
		t.Errorf("ptsMs = %d, want the keyframe at/before 8000ms", ptsMs)
	}

	dec := ffmpeg.NewTrickplayGenerator("ffmpeg", "ffprobe")
	jpeg, err := dec.DecodePacketToJPEG(ctx, packet, ext, 160)
	if err != nil {
		t.Fatalf("DecodePacketToJPEG: %v", err)
	}
	// JPEG SOI marker.
	if !bytes.HasPrefix(jpeg, []byte{0xFF, 0xD8}) {
		t.Errorf("decoded output is not a JPEG (no SOI marker), got %d bytes", len(jpeg))
	}
}
