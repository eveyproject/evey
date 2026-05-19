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
	"os"

	"github.com/buckket/go-blurhash"
)

// blurhashComponents is the (x, y) tuple the encoder uses to generate the
// compact placeholder. 4×3 is the Wolt / Jellyfin default for landscape-ish
// covers and produces strings ~30 characters long — small enough to ship
// in every list response, blurry enough to read as a colour-only preview.
const (
	blurhashComponentsX = 4
	blurhashComponentsY = 3
)

// computeBlurhash decodes the image at path and returns its blurhash.
// Returns an error when the format is unsupported or the file is unreadable;
// the caller logs and proceeds (the placeholder is a UX nicety, not a
// correctness requirement).
func computeBlurhash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()
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
