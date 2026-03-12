package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type TrickplayGenerator struct {
	binary string
}

func NewTrickplayGenerator(binary string) *TrickplayGenerator {
	return &TrickplayGenerator{binary: binary}
}

func (g *TrickplayGenerator) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight int) (*library.TrickplayResult, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create trickplay dir: %w", err)
	}

	outputPattern := filepath.Join(outputDir, "trickplay_%03d.jpg")
	vf := fmt.Sprintf("fps=1/%d,scale=%d:-1,tile=%dx%d", interval, width, tileWidth, tileHeight)

	cmd := exec.CommandContext(ctx, g.binary,
		"-skip_frame", "noref",
		"-i", filePath,
		"-vf", vf,
		"-vsync", "vfr",
		"-q:v", "5",
		"-an",
		outputPattern,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg trickplay: %w\n%s", err, string(out))
	}

	imageCount := 0
	for i := 1; ; i++ {
		p := filepath.Join(outputDir, fmt.Sprintf("trickplay_%03d.jpg", i))
		if _, statErr := os.Stat(p); statErr != nil {
			break
		}
		imageCount++
	}

	return &library.TrickplayResult{ //nolint:nilerr // os.Stat error means file not found; returning nil is correct
		ImageDir:   outputDir,
		ImageCount: imageCount,
	}, nil
}
