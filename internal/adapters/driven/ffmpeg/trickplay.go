package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// keyframeInputFormat maps a mkvgo keyframe-packet extension to the ffmpeg
// demuxer that reads it: Annex-B elementary streams for H.264/HEVC, an IVF
// container for VP8/VP9/AV1.
var keyframeInputFormat = map[string]string{
	".h264": "h264",
	".hevc": "hevc",
	".ivf":  "ivf",
}

// TrickplayWorkers returns the ffmpeg internal thread count for a single
// trickplay generation, sized off sysinfo.CPUBudget() (the cgroup CPU quota
// when one is set, else runtime.NumCPU()) rather than the host's raw core
// count, so a container with a CPU limit doesn't oversubscribe.
// adaptiveCPU=true -> 25% of the budget (gentle), false -> 50% (faster).
// Minimum 1, maximum 4.
func TrickplayWorkers(adaptiveCPU bool) int {
	divisor := 4
	if !adaptiveCPU {
		divisor = 2
	}
	n := sysinfo.CPUBudget() / divisor
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
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
// (0 → use the CPU-budget default; adaptive stays gentle at ~25% of budget).
func (g *TrickplayGenerator) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int) (*library.TrickplayResult, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.trickplay.Generate")
	defer span.End()
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
	// -skip_frame nokey instructs the decoder to drop non-keyframe packets
	// so HEVC/H.264 4K decode only processes I-frames (typically every
	// 2-5 seconds). On a 1080p HEVC episode this turns ~9 min of full
	// decode into ~30s of keyframe-only work — without this flag the
	// single-pass version still pegged 1 core at 100% for minutes per item.
	//
	// -vsync vfr stops ffmpeg from trying to duplicate frames to match
	// a constant framerate when the input stream has gaps (keyframe
	// skipping creates gaps by design).
	//
	// We use the `select` filter (NOT `fps=1/N`): `fps` rewrites every
	// output frame's PTS to a clean multiple of N, which is a lie when
	// keyframes are sparse — players that trust the PTS would scrub to
	// frames whose true source position differs by ±2.5s. `select` keeps
	// each frame's original PTS, so the `showinfo` filter downstream logs
	// the real source-video time for every kept thumbnail.
	//
	// Expression: pick the first frame, then any frame whose time is at
	// least N seconds after the previously selected — yields one thumb
	// per ~N seconds of source while honoring keyframe placement.
	vf := fmt.Sprintf(
		"select='isnan(prev_selected_t)+gte(t-prev_selected_t\\,%d)',scale=%d:-1,showinfo,tile=%dx%d",
		interval, width, tileWidth, tileHeight,
	)
	cmd := exec.CommandContext(ctx, "nice", "-n", "19", g.binary,
		"-nostdin",
		"-threads", strconv.Itoa(threads),
		"-skip_frame", "nokey",
		"-i", filePath,
		"-an",
		"-vsync", "vfr",
		"-vf", vf,
		"-c:v", "libwebp",
		"-quality", "75",
		"-y", outputPattern,
	)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return nil, fmt.Errorf("ffmpeg trickplay single-pass: %w\n%s", runErr, string(out))
	}

	timestampsMs := parseShowinfoTimestamps(string(out))
	if len(timestampsMs) == 0 {
		return nil, errors.New("trickplay: no timestamps captured from showinfo")
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
		ImageDir:          outputDir,
		ImageCount:        imageCount,
		FrameTimestampsMs: timestampsMs,
	}, nil
}

// reShowinfoPts matches the pts_time field in showinfo lines.
// Example: "[Parsed_showinfo_2 @ 0x...] n: 1 pts: 7672500 pts_time:7.672 ..."
var reShowinfoPts = regexp.MustCompile(`pts_time:(\d+(?:\.\d+)?)`)

// parseShowinfoTimestamps pulls every pts_time value from ffmpeg's stderr
// (showinfo output) and returns them as milliseconds, in extraction order.
func parseShowinfoTimestamps(stderr string) []int64 {
	matches := reShowinfoPts.FindAllStringSubmatch(stderr, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]int64, 0, len(matches))
	for _, m := range matches {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		out = append(out, int64(v*1000))
	}
	return out
}

// ExtractFrame extracts a single JPEG frame at the given position (seconds).
// Returns the raw JPEG bytes. Uses -ss before -i for fast seeking.
func (g *TrickplayGenerator) ExtractFrame(ctx context.Context, filePath string, positionSec float64, width int) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.trickplay.ExtractFrame")
	defer span.End()
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

// DecodePacketToJPEG decodes a single encoded keyframe packet (from mkvgo's
// ExtractKeyframeSample — Annex-B or IVF) into a scaled JPEG. It decodes just
// that one frame, so on a weak NAS it avoids the seek + GOP decode ExtractFrame
// pays. ext selects the input demuxer (.h264/.hevc/.ivf); an unknown ext errors
// so the caller falls back to the exact ffmpeg -ss path.
func (g *TrickplayGenerator) DecodePacketToJPEG(ctx context.Context, packet []byte, ext string, width int) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.trickplay.DecodePacketToJPEG")
	defer span.End()

	inFmt := keyframeInputFormat[strings.ToLower(ext)]
	if inFmt == "" {
		return nil, fmt.Errorf("decode keyframe packet: unsupported ext %q", ext)
	}
	cmd := exec.CommandContext(ctx, g.binary,
		"-nostdin",
		"-f", inFmt,
		"-i", "pipe:0",
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-1", width),
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "3",
		"-y", "pipe:1",
	)
	cmd.Stdin = bytes.NewReader(packet)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg decode keyframe packet (%s): %w", inFmt, err)
	}
	return out, nil
}
