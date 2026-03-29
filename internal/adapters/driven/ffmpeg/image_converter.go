package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
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
