package ffmpeg

import (
	"context"
	"fmt"

	"github.com/eveyproject/evey/internal/pkg/procfork"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PhotoTranscoder resizes/reformats photos via ffmpeg for thumbnail serving.
type PhotoTranscoder struct {
	binary string
}

func NewPhotoTranscoder(binary string) *PhotoTranscoder {
	if binary == "" {
		binary = "ffmpeg"
	}
	return &PhotoTranscoder{binary: binary}
}

// Transcode resizes srcPath to width (pixels, no upscale) and writes to dstPath.
// If format == "webp", encodes via libwebp. Otherwise keeps source codec (mjpeg).
// width <= 0 means no resize.
func (p *PhotoTranscoder) Transcode(ctx context.Context, srcPath, dstPath string, width int, format string) error {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.photo.Transcode")
	defer span.End()
	args := []string{"-y", "-i", srcPath}
	if width > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale='min(%d,iw)':-1", width))
	}
	switch format {
	case "webp":
		args = append(args, "-c:v", "libwebp", "-quality", "80", "-compression_level", "4")
	case "jpeg", "jpg", "":
		args = append(args, "-c:v", "mjpeg", "-q:v", "4")
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
	args = append(args, dstPath)
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Interactive, Site: "photo.transcode"}, p.binary, args...)
	if err != nil {
		return fmt.Errorf("ffmpeg photo transcode: %w\n%s", err, out)
	}
	return nil
}
