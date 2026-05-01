package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

type ImageConverter struct {
	binary string
}

func NewImageConverter(binary string) *ImageConverter {
	return &ImageConverter{binary: binary}
}

func (c *ImageConverter) ConvertToWebP(ctx context.Context, srcPath, dstPath string, quality int) error {
	if quality <= 0 {
		quality = 80
	}
	cmd := exec.CommandContext(ctx, c.binary,
		"-i", srcPath,
		"-c:v", "libwebp",
		"-quality", fmt.Sprintf("%d", quality),
		"-y", dstPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg webp convert: %w\n%s", err, out)
	}
	return nil
}

// ResizeAndConvertToWebP scales src to fit width (height auto, ratio preserved)
// and encodes WebP at quality. width=0 delegates to ConvertToWebP (no scale filter).
func (c *ImageConverter) ResizeAndConvertToWebP(ctx context.Context, srcPath, dstPath string, width, quality int) error {
	if width <= 0 {
		return c.ConvertToWebP(ctx, srcPath, dstPath, quality)
	}
	if quality <= 0 {
		quality = 80
	}
	// scale=WIDTH:-2 keeps ratio; -2 rounds height to nearest even (required by some codecs, harmless for libwebp).
	args := []string{
		"-y", "-loglevel", "error",
		"-i", srcPath,
		"-vf", fmt.Sprintf("scale=%d:-2:flags=lanczos", width),
		"-c:v", "libwebp",
		"-quality", strconv.Itoa(quality),
		"-compression_level", "4",
		dstPath,
	}
	cmd := exec.CommandContext(ctx, c.binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg resize+webp: %w (%s)", err, stderr.String())
	}
	return nil
}
