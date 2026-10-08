package photo

import (
	"fmt"
	"image"
	// Blank imports register decoders so image.Decode auto-detects the
	// format. JPEG is the dominant photo encoding; PNG covers screenshots
	// and graphics; GIF is rare but cheap. HEIC / RAW are intentionally
	// out - they need cgo libs and the encoder falls back gracefully.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"github.com/bbrks/go-blurhash"
)

// blurhashComponentsX / blurhashComponentsY size the encoder's frequency
// resolution. 4×3 is the Wolt default for landscape covers and produces
// ~30-char strings - enough detail to read the image's dominant colours
// without inflating every list response.
const (
	blurhashComponentsX = 4
	blurhashComponentsY = 3
)

// maxPhotoPixels caps the decoded frame so a "pixel flood" image (small
// file, gigapixel declared dimensions) can't OOM the scanner - image.Decode
// allocates width*height*4 bytes. The limit is higher than the artwork cap
// because real camera sensors are large (modern phones reach 50-200 MP); a
// genuine decompression bomb declares gigapixels and is still rejected. At
// 250 MP the transient allocation tops out near 1 GB, freed after encode.
const maxPhotoPixels = 250_000_000

// computePhotoBlurhash returns the blurhash of the image at path or "" plus
// an error if the file can't be decoded or is too large. Callers swallow the
// error: a missing placeholder is a UX nicety, not a scan-blocking condition.
func computePhotoBlurhash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	// Header-only read first: reject oversized images before image.Decode
	// allocates the full frame.
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return "", fmt.Errorf("decode config: %w", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPhotoPixels {
		return "", fmt.Errorf("image too large for blurhash: %dx%d", cfg.Width, cfg.Height)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek: %w", err)
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	hash, err := blurhash.Encode(blurhashComponentsX, blurhashComponentsY, img)
	if err != nil {
		return "", fmt.Errorf("encode: %w", err)
	}
	return hash, nil
}
