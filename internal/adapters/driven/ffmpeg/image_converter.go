package ffmpeg

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eveyproject/evey/internal/pkg/procfork"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ImageConverter struct {
	binary string
}

func NewImageConverter(binary string) *ImageConverter {
	return &ImageConverter{binary: binary}
}

func (c *ImageConverter) ConvertToWebP(ctx context.Context, srcPath, dstPath string, quality int) error {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.imgconv.ConvertToWebP")
	defer span.End()
	if quality <= 0 {
		quality = 80
	}
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Interactive, Site: "image_converter.webp"}, c.binary,
		"-i", srcPath,
		"-c:v", "libwebp",
		"-quality", fmt.Sprintf("%d", quality),
		"-y", dstPath,
	)
	if err != nil {
		return fmt.Errorf("ffmpeg webp convert: %w\n%s", err, out)
	}
	return nil
}

// ResizeAndConvertToWebP scales src to fit width (height auto, ratio preserved)
// and encodes WebP at quality. width=0 delegates to ConvertToWebP (no scale filter).
func (c *ImageConverter) ResizeAndConvertToWebP(ctx context.Context, srcPath, dstPath string, width, quality int) error {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.imgconv.ResizeAndConvertToWebP")
	defer span.End()
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
	if out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Interactive, Site: "image_converter.resize_webp"}, c.binary, args...); err != nil {
		return fmt.Errorf("ffmpeg resize+webp: %w (%s)", err, out)
	}
	return nil
}
