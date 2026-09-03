package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ComplexityClassifier samples a media file with ffmpeg to extract the
// metrics the per-title classifier consumes (motion mean, scenecut count,
// source bitrate / resolution). Runs as 8 small seek-and-decode bursts of
// ~1 s each - the single-pass select-filter approach OOM-killed on long
// 4K HDR files (commit f69a4a82f). Total wall-clock budget per file is
// 60 s; per burst is 15 s. Designed for an idle background worker, never
// on the playback hot path.
type ComplexityClassifier struct {
	ffmpegBin  string
	ffprobeBin string
	// SampleFrames is the number of frames we ask ffmpeg to select across
	// the duration. 30 is enough for stable YDEV mean without breaking
	// the budget; lower at extreme cases (very short files).
	SampleFrames int
	// busy, when set and true, suspends the signalstats decode while real work
	// runs (see pauseProcessWhileBusy). nil = run flat out.
	busy func() bool
}

// WithBusyChecker wires the predicate used to SIGSTOP the signalstats ffmpeg
// while a playback/transcode session or a recalibration is active. nil-safe.
func (c *ComplexityClassifier) WithBusyChecker(busy func() bool) *ComplexityClassifier {
	c.busy = busy
	return c
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
func (c *ComplexityClassifier) Classify(ctx context.Context, filePath string, durationSec float64, known transcoding.KnownSourceFacts) (transcoding.ComplexityMetrics, error) {
	metrics := transcoding.ComplexityMetrics{}
	if filePath == "" {
		return metrics, fmt.Errorf("complexity classifier: empty filePath")
	}

	// Probe the source for bitrate + resolution, unless the caller already knows
	// both - the scan establishes them, so on the normal path this fork is pure
	// duplication. ffprobe stays the fallback for whatever the caller could not
	// supply (a file scanned before the bitrate column was populated).
	var bitrateKbps int64
	var w, h int
	var err error
	if known.Complete() {
		bitrateKbps, w, h = known.BitrateBps/1000, known.Width, known.Height
	} else {
		bitrateKbps, w, h, err = c.probeSource(ctx, filePath)
	}
	if err != nil {
		// Don't fail the whole classification on a probe error - the
		// metrics struct still carries useful YDEV / scenecut info from
		// the sample pass below, and the worker can store a partial row.
		// Logging happens at the caller's slog scope.
		_ = err
	}
	metrics.SourceBitrateKbps = bitrateKbps
	metrics.SourceWidth = w
	metrics.SourceHeight = h

	// Per-frame YDEV / scenecut signals are nice-to-have but the original
	// single-pass `select=not(mod(n,X))` pipeline forced ffmpeg to decode
	// every input frame to compute mod() - 4K HDR sources OOM-killed
	// inside 90s on the dev box. Replace with a small number of seek-and-
	// sample short bursts: each ffmpeg invocation hits ~1 second of video
	// at a calibrated position, runs signalstats on the few decoded
	// frames, prints the per-frame YDEV. Total wallclock for 8 samples
	// ~5-15 s on the dev box; bounded memory because each invocation
	// decodes ~24 frames not the full film.
	//
	// Falls back gracefully: probe-only (bitrate + resolution) metrics
	// are already populated above, so a classifier failure here still
	// yields a usable metrics struct and the domain Classify()
	// decision table copes via its bitrate/resolution branches.
	sample := c.SampleFrames
	if sample <= 0 {
		sample = 8
	}
	if sample > 16 {
		sample = 16
	}
	// Don't sample inside the first / last 5 % of the file - opening
	// credits and end credits skew the metrics in unhelpful ways.
	startSec := durationSec * 0.05
	endSec := durationSec * 0.95
	if endSec <= startSec || endSec < 5 {
		sample = 1
		startSec = 0
		endSec = durationSec
	}

	var totalMotionSum float64
	var totalMotionN, totalCuts int

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for i := range sample {
		if runCtx.Err() != nil {
			break
		}
		ratio := float64(i) / float64(sample-1)
		if sample == 1 {
			ratio = 0.5
		}
		seek := startSec + ratio*(endSec-startSec)
		motionSum, motionN, cuts := c.sampleAt(runCtx, filePath, seek)
		totalMotionSum += motionSum
		totalMotionN += motionN
		totalCuts += cuts
	}

	metrics.FramesSampled = totalMotionN
	if totalMotionN > 0 {
		metrics.MotionMean = totalMotionSum / float64(totalMotionN)
	}
	if durationSec > 0 && totalMotionN > 0 {
		// Cuts observed across the ~sample-second bursts, scaled to "per
		// minute" assuming the bursts are representative of the whole
		// film. Coarse but enough to bucket animation vs sport.
		burstSeconds := float64(sample) // each burst ~1 s
		metrics.ScenecutsPerMinute = float64(totalCuts) * 60.0 / burstSeconds
	}
	return metrics, nil
}

// sampleAt seeks to `atSec`, decodes ~1 second of video at that point, and
// runs signalstats on the decoded frames. Returns (sumYDEV, count,
// scenecuts) from the metadata stream. Best-effort: any ffmpeg error
// yields zeros and is silently swallowed (the caller still has probe
// metrics to work with). Bounded by a 15 s wallclock per burst.
func (c *ComplexityClassifier) sampleAt(parent context.Context, filePath string, atSec float64) (float64, int, int) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	// Background sampler: throttle like the other off-path ffmpeg probes
	// (trickplay, intro detection) so a classification burst can't preempt a
	// live software transcode on a weak NAS - the very starvation the QoE
	// pipeline measures. nice -n 19 + a bounded decode thread count.
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "complexity.sample"}, c.ffmpegBin,
		"-hide_banner", "-nostats",
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", fmt.Sprintf("%.3f", atSec),
		"-i", filePath,
		"-t", "1",
		// signalstats → YDIF (motion proxy), scdet → scd.score (scene-change
		// score on every frame). Both share the decoded-frame stream so the
		// added cost is filter-arithmetic only, not extra decoding -
		// compatible with the 8-burst OOM budget. The previous filtergraph
		// used `select='gt(scene,0.3)'` which forced re-decoding scene-only
		// and was removed for OOM (f69a4a82f), but never updated the regex
		// downstream → scenecuts silently stayed at 0 and ClassSport became
		// unreachable. scdet emits scd.score continuously so we just need to
		// threshold in parseSignalStats.
		"-vf", "signalstats,scdet,metadata=print:file=-",
		"-an", "-sn",
		"-f", "null", "-",
	)
	if err != nil {
		return 0, 0, 0
	}
	var stderr, stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	// Best-effort sampler: ffmpeg failures (codec issues, oversized
	// input, timeout) all yield "no samples here", which parseSignalStats
	// handles via the FramesSampled==0 fallback. We deliberately don't
	// propagate the error to the caller.
	_ = finish(runPausableCmd(cmd, c.busy), stderr.Bytes()) //nolint:errcheck // best-effort sampler, see above
	return parseSignalStats(stdout.String() + "\n" + stderr.String())
}

// probeSource pulls the format-level bitrate + the first video stream's
// resolution. ffprobe -of csv would be terse but the parsing is fiddly;
// -of default lets us regexp out the three fields we need.
func (c *ComplexityClassifier) probeSource(ctx context.Context, filePath string) (bitrateKbps int64, width, height int, err error) {
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, e := procfork.Output(runCtx, procfork.Options{Class: procfork.Probe, Site: "complexity.probe_source"}, c.ffprobeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "format=bit_rate:stream=width,height",
		"-of", "default=noprint_wrappers=1",
		filePath,
	)
	if e != nil {
		return 0, 0, 0, fmt.Errorf("ffprobe: %w", e)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
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

// parseSignalStats walks the combined stdout/stderr of the ffmpeg call
// and pulls (sum of motion, frame count, scenecut count). metadata=print
// writes one block per decoded frame; the filtergraph emits
// `lavfi.signalstats.YDIF=...` per frame (motion proxy) and
// `lavfi.scd.score=...` per frame (scene-change score, thresholded at
// 0.3 here to match the typical scdet default).
//
// The original implementation grepped YDEV (never emitted by signalstats
// - only YDIF/YAVG/YMIN/YHIGH/YMAX exist) and lavfi.scd.score without the
// scdet filter chained in - both regexes silently matched zero, so the
// motion mean and scenecut count both stayed at 0 on every classify.
// Fixed in two passes: regex switch to YDIF, then filtergraph addition
// of scdet so the scd.score regex actually has input to match.
//
// Best-effort: garbled output yields a partial parse with low FramesSampled
// rather than crashing.
var ydifRE = regexp.MustCompile(`lavfi\.signalstats\.YDIF=([0-9]+\.?[0-9]*)`)
var sceneRE = regexp.MustCompile(`lavfi\.scd\.score=([0-9]+\.?[0-9]*)`)

func parseSignalStats(text string) (motionSum float64, frameCount int, sceneCuts int) {
	for _, m := range ydifRE.FindAllStringSubmatch(text, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			motionSum += v
			frameCount++
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
