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

func TestNewPhotoTranscoder_DefaultBinary(t *testing.T) {
	p := NewPhotoTranscoder("")
	if p.binary != "ffmpeg" {
		t.Errorf("binary = %q, want ffmpeg", p.binary)
	}
}

func TestNewPhotoTranscoder_ExplicitBinary(t *testing.T) {
	p := NewPhotoTranscoder("/usr/local/bin/ffmpeg")
	if p.binary != "/usr/local/bin/ffmpeg" {
		t.Errorf("binary = %q, want explicit path", p.binary)
	}
}

func TestPhotoTranscoder_UnsupportedFormat(t *testing.T) {
	p := NewPhotoTranscoder("ffmpeg")
	err := p.Transcode(context.Background(), "/tmp/src.jpg", "/tmp/dst.x", 0, "heif")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestPhotoTranscoder_BogusBinary(t *testing.T) {
	p := NewPhotoTranscoder("ffmpeg-does-not-exist-xyz")
	err := p.Transcode(context.Background(), "/tmp/src.jpg", "/tmp/dst.webp", 100, "webp")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestPhotoTranscoder_Transcode_JPEG(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "in.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "out.jpg")

	p := NewPhotoTranscoder("ffmpeg")
	if err := p.Transcode(context.Background(), src, dst, 2, "jpeg"); err != nil {
		t.Fatalf("Transcode jpeg: %v", err)
	}
	fi, err := os.Stat(dst)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("jpeg missing or empty: err=%v", err)
	}
}

func TestPhotoTranscoder_Transcode_WebP_NoResize(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "in.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "out.webp")

	p := NewPhotoTranscoder("ffmpeg")
	if err := p.Transcode(context.Background(), src, dst, 0, "webp"); err != nil {
		t.Fatalf("Transcode webp: %v", err)
	}
	fi, err := os.Stat(dst)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("webp missing or empty: err=%v", err)
	}
}
