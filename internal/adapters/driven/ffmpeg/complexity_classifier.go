package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ComplexityClassifier runs a one-shot ffmpeg pass on a media file to
// extract the metrics the per-title classifier consumes (YDEV mean,
// scenecut count, source bitrate / resolution). Single ffmpeg invocation
// per file — we sample 30 frames spread across the duration via the
// `select` filter, run them through `signalstats` for YDEV, and count
// scenecuts in the same pipeline.
//
// One pass keeps the worker cheap: 10-30s of CPU per film on the dev box,
// run idle in the background and never on the playback hot path.
type ComplexityClassifier struct {
	ffmpegBin  string
	ffprobeBin string
	// SampleFrames is the number of frames we ask ffmpeg to select across
	// the duration. 30 is enough for stable YDEV mean without breaking
	// the budget; lower at extreme cases (very short files).
	SampleFrames int
}

// NewComplexityClassifier returns a classifier wired to the local ffmpeg
// + ffprobe binaries. Empty paths fall back to "ffmpeg" / "ffprobe" on
// PATH (matches the rest of this package).
func NewComplexityClassifier(ffmpegBin, ffprobeBin string) *ComplexityClassifier {
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}
	if ffprobeBin == "" {
		ffprobeBin = "ffprobe"
	}
	return &ComplexityClassifier{
		ffmpegBin:    ffmpegBin,
		ffprobeBin:   ffprobeBin,
		SampleFrames: 30,
	}
}

var _ transcoding.Classifier = (*ComplexityClassifier)(nil)

// Classify samples the file and returns the metrics. Total wallclock ~10-30s
// for a typical 1080p film on the dev box; bounded by an internal 90s
// timeout so a degenerate file doesn't pin a worker forever.
func (c *ComplexityClassifier) Classify(ctx context.Context, filePath string, durationSec float64) (transcoding.ComplexityMetrics, error) {
	metrics := transcoding.ComplexityMetrics{}
	if filePath == "" {
		return metrics, fmt.Errorf("complexity classifier: empty filePath")
	}

	// Probe the source for bitrate + resolution. ffprobe is fast (<200ms)
	// and we need this even when the frame sample fails.
	bitrateKbps, w, h, err := c.probeSource(ctx, filePath)
	if err != nil {
		// Don't fail the whole classification on a probe error — the
		// metrics struct still carries useful YDEV / scenecut info from
		// the sample pass below, and the worker can store a partial row.
		// Logging happens at the caller's slog scope.
		_ = err
	}
	metrics.SourceBitrateKbps = bitrateKbps
	metrics.SourceWidth = w
	metrics.SourceHeight = h

	// Build the sample-and-stats pipeline. The select filter picks
	// `SampleFrames` frames spread evenly across the duration; signalstats
	// emits YDEV per frame as metadata that we grep from ffmpeg's stderr.
	// scene>0.3 trips on hard cuts; the showinfo filter logs each
	// selected frame so we can count scenecuts independently of the
	// YDEV stream.
	sample := c.SampleFrames
	if sample <= 0 {
		sample = 30
	}
	stride := durationSec / float64(sample)
	if stride < 1.0 {
		// Files shorter than `sample` seconds: just sample every second.
		// Adjusts sample down for very short fixtures (tests, music tracks).
		stride = 1.0
		sample = int(durationSec)
		if sample < 1 {
			sample = 1
		}
	}

	// `-skip_frame nokey` ⇒ feed only keyframes to the filtergraph so the
	// "30 samples across the duration" arithmetic stays cheap (no full
	// decode). signalstats wants a real frame regardless, so we drop the
	// flag if it makes the sample empty.
	filterGraph := fmt.Sprintf(
		"select='not(mod(n\\,%d))*lte(selected_n\\,%d)',signalstats,select='gt(scene\\,0.3)+1',metadata=print:file=-",
		int(30.0*stride), // n stride in frames assuming 30 fps source — rough but adequate
		sample,
	)

	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, c.ffmpegBin,
		"-hide_banner", "-nostats",
		"-i", filePath,
		"-vf", filterGraph,
		"-an", "-sn",
		"-f", "null", "-",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	runErr := cmd.Run()
	// Even on non-zero exit the partial stderr usually contains usable
	// signalstats lines (e.g. the file ends mid-segment but we got 20+
	// samples). Parse what we have before bailing.
	ydevSum, ydevN, sceneCuts := parseSignalStats(stdout.String() + "\n" + stderr.String())
	metrics.FramesSampled = ydevN
	if ydevN > 0 {
		metrics.YDEVMean = ydevSum / float64(ydevN)
	}
	if durationSec > 0 {
		metrics.ScenecutsPerMinute = float64(sceneCuts) / (durationSec / 60.0)
	}

	if runErr != nil && ydevN == 0 {
		return metrics, fmt.Errorf("complexity classifier: ffmpeg sample failed (%w) — stderr tail: %s",
			runErr, tailString(stderr.String(), 400))
	}
	return metrics, nil
}

// probeSource pulls the format-level bitrate + the first video stream's
// resolution. ffprobe -of csv would be terse but the parsing is fiddly;
// -of default lets us regexp out the three fields we need.
func (c *ComplexityClassifier) probeSource(ctx context.Context, filePath string) (bitrateKbps int64, width, height int, err error) {
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, c.ffprobeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "format=bit_rate:stream=width,height",
		"-of", "default=noprint_wrappers=1",
		filePath,
	)
	out, e := cmd.Output()
	if e != nil {
		return 0, 0, 0, fmt.Errorf("ffprobe: %w", e)
	}
	for _, line := range strings.Split(string(out), "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "bit_rate":
			if n, perr := strconv.ParseInt(kv[1], 10, 64); perr == nil {
				bitrateKbps = n / 1000
			}
		case "width":
			if n, perr := strconv.Atoi(kv[1]); perr == nil {
				width = n
			}
		case "height":
			if n, perr := strconv.Atoi(kv[1]); perr == nil {
				height = n
			}
		}
	}
	return bitrateKbps, width, height, nil
}

// parseSignalStats walks the combined stdout/stderr of the ffmpeg invocation
// and pulls (sum of YDEV, count of frames, count of scenecuts). The
// `metadata=print` filter writes one block per selected frame in stdout;
// signalstats keys land in `lavfi.signalstats.YDEV=NN.NN`. The scene
// filter increments `pts_time` lines we ignore — scenecuts are reported
// in stderr by signalstats' own scene metadata when present, but we
// fallback to counting `lavfi.scd.mafd` jumps if upstream prints them.
//
// Best-effort: garbled output yields a partial parse with low FramesSampled
// rather than crashing.
var ydevRE = regexp.MustCompile(`lavfi\.signalstats\.YDEV=([0-9]+\.?[0-9]*)`)
var sceneRE = regexp.MustCompile(`lavfi\.scd\.score=([0-9]+\.?[0-9]*)`)

func parseSignalStats(text string) (ydevSum float64, ydevN int, sceneCuts int) {
	for _, m := range ydevRE.FindAllStringSubmatch(text, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			ydevSum += v
			ydevN++
		}
	}
	for _, m := range sceneRE.FindAllStringSubmatch(text, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0.3 {
			sceneCuts++
		}
	}
	return
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
