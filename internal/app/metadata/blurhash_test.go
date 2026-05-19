package metadata

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeTestPNG(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestComputeBlurhash_PNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "solid.png")
	writeTestPNG(t, path, color.RGBA{R: 200, G: 50, B: 100, A: 255})

	hash, err := computeBlurhash(path)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if hash == "" {
		t.Error("hash should be non-empty for a valid image")
	}
	// Blurhash strings start with a length-prefix character; the rest is the
	// encoded payload. ~30 chars for 4×3 components is typical.
	if len(hash) < 20 {
		t.Errorf("hash unexpectedly short (%d): %q", len(hash), hash)
	}
}

func TestComputeBlurhash_FileMissing(t *testing.T) {
	_, err := computeBlurhash(filepath.Join(t.TempDir(), "does-not-exist.png"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestComputeBlurhash_NotAnImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.bin")
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := computeBlurhash(path)
	if err == nil {
		t.Error("expected decode error for non-image input")
	}
}
