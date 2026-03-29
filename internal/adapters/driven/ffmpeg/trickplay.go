package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/pkg/cpuutil"
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

func (g *TrickplayGenerator) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int) (*library.TrickplayResult, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create trickplay dir: %w", err)
	}

	duration, err := g.probeDuration(ctx, filePath)
	if err != nil {
		return nil, err
	}

	var timestamps []float64
	for t := 0.0; t < duration; t += float64(interval) {
		timestamps = append(timestamps, t)
	}
	if len(timestamps) == 0 {
		timestamps = []float64{0}
	}

	thumbDir, err := os.MkdirTemp(outputDir, "thumbs-")
	if err != nil {
		return nil, fmt.Errorf("create thumb dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(thumbDir); rmErr != nil {
			slog.Warn("trickplay: remove thumb dir", slog.String("dir", thumbDir), slog.Any("err", rmErr))
		}
	}()

	workers := maxWorkers
	if workers <= 0 {
		workers = TrickplayWorkers(true)
	}
	sem := make(chan struct{}, workers)
	errs := make([]error, len(timestamps))
	var wg sync.WaitGroup
	for i, ts := range timestamps {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, t float64) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in trickplay frame extract", "error", r, "stack", debug.Stack())
				}
			}()
			out := filepath.Join(thumbDir, fmt.Sprintf("thumb_%05d.png", idx))
			cmd := exec.CommandContext(ctx, "nice", "-n", "19", g.binary,
				"-threads", "1",
				"-ss", fmt.Sprintf("%.3f", t),
				"-i", filePath,
				"-frames:v", "1",
				"-vf", fmt.Sprintf("scale=%d:-1", width),
				"-y", out,
			)
			if b, runErr := cmd.CombinedOutput(); runErr != nil {
				errs[idx] = fmt.Errorf("thumb %d at %.3fs: %w\n%s", idx, t, runErr, b)
			}
		}(i, ts)
	}
	wg.Wait()

	// Build concat list from successfully extracted thumbs.
	concatPath := filepath.Join(thumbDir, "concat.txt")
	cf, err := os.Create(concatPath)
	if err != nil {
		return nil, fmt.Errorf("create concat: %w", err)
	}
	defer cf.Close()
	existing := 0
	for i := range timestamps {
		p := filepath.Join(thumbDir, fmt.Sprintf("thumb_%05d.png", i))
		if _, statErr := os.Stat(p); statErr == nil {
			fmt.Fprintf(cf, "file '%s'\nduration 1\n", p)
			existing++
		}
	}

	if existing == 0 {
		return nil, fmt.Errorf("no thumbnails extracted")
	}

	// Tile thumbnails into sprite sheets.
	// -r 1 forces 1fps so each concat entry (duration 1) = exactly 1 frame for the tile filter.
	outputPattern := filepath.Join(outputDir, "trickplay_%03d.webp")
	tileCmd := exec.CommandContext(ctx, "nice", "-n", "19", g.binary,
		"-threads", "1",
		"-f", "concat",
		"-safe", "0",
		"-r", "1",
		"-i", concatPath,
		"-vf", fmt.Sprintf("tile=%dx%d", tileWidth, tileHeight),
		"-c:v", "libwebp",
		"-quality", "75",
		"-y", outputPattern,
	)
	if out, runErr := tileCmd.CombinedOutput(); runErr != nil {
		return nil, fmt.Errorf("ffmpeg tile: %w\n%s", runErr, string(out))
	}

	imageCount := 0
	for i := 1; ; i++ {
		p := filepath.Join(outputDir, fmt.Sprintf("trickplay_%03d.webp", i))
		if _, statErr := os.Stat(p); statErr != nil {
			break
		}
		imageCount++
	}

	return &library.TrickplayResult{
		ImageDir:   outputDir,
		ImageCount: imageCount,
	}, nil
}
