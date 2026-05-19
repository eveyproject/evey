package photo

import (
	"fmt"
	"image"
	// Blank imports register decoders so image.Decode auto-detects the
	// format. JPEG is the dominant photo encoding; PNG covers screenshots
	// and graphics; GIF is rare but cheap. HEIC / RAW are intentionally
	// out — they need cgo libs and the encoder falls back gracefully.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/buckket/go-blurhash"
)

// blurhashComponentsX / blurhashComponentsY size the encoder's frequency
// resolution. 4×3 is the Wolt default for landscape covers and produces
// ~30-char strings — enough detail to read the image's dominant colours
// without inflating every list response.
const (
	blurhashComponentsX = 4
	blurhashComponentsY = 3
)

// computePhotoBlurhash returns the blurhash of the image at path or "" plus
// an error if the file can't be decoded. Callers swallow the error: a
// missing placeholder is a UX nicety, not a scan-blocking condition.
func computePhotoBlurhash(path string) (string, error) {
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
