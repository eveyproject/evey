package metadata

import (
	"fmt"
	"image"
	// Blank imports register the JPEG/PNG/GIF decoders with image.Decode.
	// TMDB ships posters/backdrops as JPEG; PNG covers logos + clear art;
	// GIF is rare but cheap to include.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"github.com/bbrks/go-blurhash"
)

// blurhashComponents is the (x, y) tuple the encoder uses to generate the
// compact placeholder. 4×3 is the blurhash reference default for landscape-ish
// covers and produces strings ~30 characters long - small enough to ship
// in every list response, blurry enough to read as a colour-only preview.
const (
	blurhashComponentsX = 4
	blurhashComponentsY = 3
)

// maxBlurhashPixels caps the decoded frame size. image.Decode allocates
// width*height*4 bytes and the blurhash encoder iterates every pixel, so an
// untrusted "pixel flood" image (small file, enormous declared dimensions)
// would OOM the box. 50 MP is far above any real poster/backdrop (a 4K frame
// is ~8 MP) while still rejecting decompression bombs.
const maxBlurhashPixels = 50_000_000

// computeBlurhash decodes the image at path and returns its blurhash.
// Returns an error when the format is unsupported, the file is unreadable,
// or the image is too large; the caller logs and proceeds (the placeholder
// is a UX nicety, not a correctness requirement).
func computeBlurhash(path string) (string, error) {
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
	if int64(cfg.Width)*int64(cfg.Height) > maxBlurhashPixels {
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
