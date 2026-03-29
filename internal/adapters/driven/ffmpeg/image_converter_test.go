package ffmpeg

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestConvertToWebP_DefaultQuality(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "test.png")
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(tmp, "test.webp")
	conv := NewImageConverter("ffmpeg")
	if err := conv.ConvertToWebP(context.Background(), src, dst, 0); err != nil {
		t.Fatalf("ConvertToWebP: %v", err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat webp: %v", err)
	}
	if fi.Size() == 0 {
		t.Error("webp file is empty")
	}
}

func TestConvertToWebP_BadSource(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	conv := NewImageConverter("ffmpeg")
	err := conv.ConvertToWebP(context.Background(), "/nonexistent.png", "/tmp/out.webp", 80)
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}
