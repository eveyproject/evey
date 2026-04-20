package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/cpuutil"
)

// TrickplayWorkers delegates to cpuutil.TrickplayWorkers.
// Kept for backward compatibility within the adapter.
func TrickplayWorkers(adaptiveCPU bool) int {
	return cpuutil.TrickplayWorkers(adaptiveCPU)
}

type TrickplayGenerator struct {
	binary      string
	probeBinary string
}

func NewTrickplayGenerator(binary, probeBinary string) *TrickplayGenerator {
	return &TrickplayGenerator{binary: binary, probeBinary: probeBinary}
}

func (g *TrickplayGenerator) probeDuration(ctx context.Context, filePath string) (float64, error) {
	out, err := exec.CommandContext(ctx, "nice", "-n", "19", g.probeBinary,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if unmarshalErr := json.Unmarshal(out, &result); unmarshalErr != nil {
		return 0, fmt.Errorf("ffprobe parse: %w", unmarshalErr)
	}
	d, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration %q: %w", result.Format.Duration, err)
	}
	return d, nil
}

// Generate produces a trickplay sprite sheet (tiled webp images) for the
// given media file. Previous implementation launched one ffmpeg process per
// thumbnail (N seeks into the same file), which for a 2160p HEVC episode
// turned ~420 thumbs into ~25 minutes of CPU — the server spent entire
// weekends catching up on a modest library. This single-pass version
// streams the decoder once with `fps=1/interval,scale,tile` baked into
// the filter graph: the file is opened once, frames are downsampled on
// the fly, and tiled sprites land straight into the output dir as webp.
// Typical speedup: 5-15× on 4K content vs the N-call approach.
//
// maxWorkers now caps ffmpeg's internal decode/encode thread count
// (0 → use cpuutil default; adaptive stays gentle at ~25% of cores).
func (g *TrickplayGenerator) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int) (*library.TrickplayResult, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create trickplay dir: %w", err)
	}

	// Validate file duration is readable; also lets us short-circuit on
	// unprobeable inputs before spending ffmpeg startup cost.
	if _, err := g.probeDuration(ctx, filePath); err != nil {
		return nil, err
	}

	if interval <= 0 {
		interval = 10
	}
	if width <= 0 {
		width = 320
	}
	if tileWidth <= 0 {
		tileWidth = 10
	}
	if tileHeight <= 0 {
		tileHeight = 10
	}

	threads := maxWorkers
	if threads <= 0 {
		threads = TrickplayWorkers(true)
	}

	outputPattern := filepath.Join(outputDir, "trickplay_%03d.webp")
	// fps=1/interval downsamples to 1 frame per <interval> seconds of
	// source. scale preserves aspect ratio. tile packs tileW*tileH
	// frames per output sprite; ffmpeg writes sequential files
	// trickplay_001.webp, _002.webp, ... until the stream ends.
	vf := fmt.Sprintf("fps=1/%d,scale=%d:-1,tile=%dx%d", interval, width, tileWidth, tileHeight)
	cmd := exec.CommandContext(ctx, "nice", "-n", "19", g.binary,
		"-nostdin",
		"-threads", strconv.Itoa(threads),
		"-i", filePath,
		"-an",
		"-vf", vf,
		"-c:v", "libwebp",
		"-quality", "75",
		"-y", outputPattern,
	)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		return nil, fmt.Errorf("ffmpeg trickplay single-pass: %w\n%s", runErr, string(out))
	}

	imageCount := 0
	for i := 1; ; i++ {
		p := filepath.Join(outputDir, fmt.Sprintf("trickplay_%03d.webp", i))
		if _, statErr := os.Stat(p); statErr != nil {
			break
		}
		imageCount++
	}
	if imageCount == 0 {
		return nil, errors.New("no trickplay sprites produced")
	}

	return &library.TrickplayResult{
		ImageDir:   outputDir,
		ImageCount: imageCount,
	}, nil
}

// ExtractFrame extracts a single JPEG frame at the given position (seconds).
// Returns the raw JPEG bytes. Uses -ss before -i for fast seeking.
func (g *TrickplayGenerator) ExtractFrame(ctx context.Context, filePath string, positionSec float64, width int) ([]byte, error) {
	ss := fmt.Sprintf("%.3f", positionSec)
	cmd := exec.CommandContext(ctx, g.binary,
		"-ss", ss,
		"-i", filePath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-1", width),
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "3",
		"-y", "pipe:1",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg extract frame at %ss: %w", ss, err)
	}
	return out, nil
}
