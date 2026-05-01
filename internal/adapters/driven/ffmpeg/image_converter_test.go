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
	"strings"
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

func TestResizeAndConvertToWebP_Dimensions(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not in PATH")
	}

	// Create a 400×600 PNG source.
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.png")
	img := image.NewRGBA(image.Rect(0, 0, 400, 600))
	for y := range 600 {
		for x := range 400 {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(tmp, "out_w200.webp")
	conv := NewImageConverter("ffmpeg")
	if err := conv.ResizeAndConvertToWebP(context.Background(), src, dst, 200, 80); err != nil {
		t.Fatalf("ResizeAndConvertToWebP: %v", err)
	}

	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("resized webp is empty")
	}

	// Verify dimensions with ffprobe.
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
		dst,
	).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	// ffprobe output: "width,height\n"
	got := strings.TrimSpace(string(out))
	if got != "200,300" {
		t.Errorf("expected dimensions 200,300 got %q", got)
	}
}

func TestResizeAndConvertToWebP_ZeroWidth_DelegatesToConvert(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "test.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(tmp, "out.webp")
	conv := NewImageConverter("ffmpeg")
	if err := conv.ResizeAndConvertToWebP(context.Background(), src, dst, 0, 80); err != nil {
		t.Fatalf("ResizeAndConvertToWebP(width=0): %v", err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Size() == 0 {
		t.Error("output webp is empty")
	}
}
