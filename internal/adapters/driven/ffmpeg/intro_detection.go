package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/eveyproject/evey/internal/pkg/procfork"

	"golang.org/x/image/webp"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// IntroDetector performs pHash-based intro detection and FFmpeg audio/black passes.
type IntroDetector struct {
	binary      string
	probeBinary string
	// busy, when set and true, means real work (playback/transcode or a
	// recalibration) is running and a detection decode must yield. nil = run flat
	// out (the historic behaviour). See runPausable.
	busy func() bool
	// subtitleCues, when set, answers SubtitleCueStarts from the container's
	// index (mkvgo, the file's head) before ffprobe reads every packet; ok=false
	// means it cannot vouch for the track and ffprobe answers.
	subtitleCues func(ctx context.Context, filePath string) (cues []int64, ok bool)
}

func NewIntroDetector(binary, probeBinary string) *IntroDetector {
	return &IntroDetector{binary: binary, probeBinary: probeBinary}
}

// WithBusyChecker wires the predicate the detector uses to suspend its ffmpeg
// while real work is active. Injected post-construction (the session counter and
// calibration service are built later). nil-safe.
func (d *IntroDetector) WithBusyChecker(busy func() bool) *IntroDetector {
	d.busy = busy
	return d
}

// WithSubtitleCueIndex wires the index reader SubtitleCueStarts tries first.
func (d *IntroDetector) WithSubtitleCueIndex(f func(ctx context.Context, filePath string) ([]int64, bool)) *IntroDetector {
	d.subtitleCues = f
	return d
}

// runPausable runs cmd, SIGSTOPping it while d.busy() reports real work and
// resuming when it clears (see runPausableCmd / pauseProcessWhileBusy). Because
// the detection sweep is serial, a suspended decode blocks its caller so the
// whole sweep naturally pauses. With no busy checker it runs unchanged.
func (d *IntroDetector) runPausable(cmd *exec.Cmd) error {
	return runPausableCmd(cmd, d.busy)
}

// endCreditsArgs builds the credits-scan argv. It is split out so the decode
// bound is testable: -ss alone used to be the only limit, which made the window
// a CONSEQUENCE of durationSec rather than something stated to ffmpeg. That
// duration comes from the database and is filled by a probe that can fail, so an
// underestimate turned the scan into a decode of the whole file.
func endCreditsArgs(filePath string, startSec, windowSec float64) []string {
	return []string{
		"-nostdin",
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(windowSec, 'f', 3, 64),
		"-i", filePath,
		"-vf", "blackdetect=d=0.5:pix_th=0.10",
		"-an",
		"-vsync", "vfr",
		"-f", "null",
		"-",
	}
}

// killedBySignal distinguishes a process the kernel (or anyone else) shot from
// the ordinary non-zero exit `-f null` produces on every run. A cancelled
// context also kills the child, so that case is excluded: it is our own doing,
// not a resource failure.
func killedBySignal(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled()
}

// ExtractTileHashes opens a trickplay sprite file and returns one perceptual
// hash per tile. Tiles are laid out in a tileWidth×tileHeight grid; each
// tile is tilePixelWidth px wide and tilePixelHeight px tall.
//
// The hash function used is pHash DCT 8×8 (kept low-frequency block of a
// 32×32 DCT). Bumping the hash function changes the value distribution and
// requires invalidating intro_fingerprint_cache rows en masse - bump
// currentPHashAlgoVersion in intro_detection_service.go in lock-step.
func (d *IntroDetector) ExtractTileHashes(spritePath string, tileWidth, tileHeight, tilePixelWidth, tilePixelHeight int) ([]uint64, error) {
	f, err := os.Open(spritePath)
	if err != nil {
		return nil, fmt.Errorf("open sprite %q: %w", spritePath, err)
	}
	defer f.Close()

	img, err := webp.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode webp %q: %w", spritePath, err)
	}

	total := tileWidth * tileHeight
	hashes := make([]uint64, 0, total)
	for row := range tileHeight {
		for col := range tileWidth {
			x0 := col * tilePixelWidth
			y0 := row * tilePixelHeight
			x1 := x0 + tilePixelWidth
			y1 := y0 + tilePixelHeight
			hashes = append(hashes, pHashDCT(img, x0, y0, x1, y1))
		}
	}
	return hashes, nil
}

// PrecisionPass runs FFmpeg silencedetect+blackdetect on [startSec-padSec, endSec+padSec].
// Returns refined (startMs, endMs). Falls back to (startMs_in, endMs_in) when FFmpeg
// finds nothing useful.
func (d *IntroDetector) PrecisionPass(ctx context.Context, filePath string, startSec, endSec, padSec float64) (startMs, endMs int64, err error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.PrecisionPass")
	defer span.End()
	begin := math.Max(0, startSec-padSec)
	duration := endSec + padSec - begin

	silences, blacks, runErr := d.runDetect(ctx, filePath, begin, duration)
	if runErr != nil {
		return int64(startSec * 1000), int64(endSec * 1000), runErr
	}

	rStart, rEnd, found := mergeTransitions(silences, blacks, begin, begin+duration)
	if !found {
		return int64(startSec * 1000), int64(endSec * 1000), nil
	}
	return int64(rStart * 1000), int64(rEnd * 1000), nil
}

// AudioFallback runs silencedetect+blackdetect on the first maxSec of the file.
// Returns (startMs, endMs, true) if a consistent transition is found.
func (d *IntroDetector) AudioFallback(ctx context.Context, filePath string, maxSec float64) (startMs, endMs int64, found bool, err error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.AudioFallback")
	defer span.End()
	silences, blacks, runErr := d.runDetect(ctx, filePath, 0, maxSec)
	if runErr != nil {
		return 0, 0, false, runErr
	}
	rStart, rEnd, ok := mergeTransitions(silences, blacks, 0, maxSec)
	if !ok {
		return 0, 0, false, nil
	}
	return int64(rStart * 1000), int64(rEnd * 1000), true, nil
}

// blackDetectArgs and silenceDetectArgs each build a SINGLE-output argv. They
// exist as separate builders because feeding -vf and -af into one `-f null`
// muxer is what took the host down on 2026-08-24: after an input seek (-ss) on
// some MKVs the audio and video timelines resume far apart, and the muxer's
// interleave queue buffers DECODED video frames (wrapped_avframe) while it
// waits for the other stream - unbounded, measured at >500 MiB/s until OOM.
// The same command with -ss 0 stays at ~140 MiB, and each half alone is
// bounded (video 141 MiB, audio 8 MiB, combined OOM - isolated on the
// incident's own files). One decoded stream per muxer, structurally.
func blackDetectArgs(filePath string, startSec, durationSec float64) []string {
	return []string{
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-i", filePath,
		"-an",
		"-vf", "blackdetect=d=0.1:pix_th=0.10",
		"-f", "null",
		"-",
	}
}

func silenceDetectArgs(filePath string, startSec, durationSec float64) []string {
	return []string{
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-i", filePath,
		"-vn",
		"-af", "silencedetect=n=-50dB:d=0.5",
		"-f", "null",
		"-",
	}
}

// runDetect runs the silence and black scans as TWO ffmpeg invocations (see the
// builders above for why they must never share a muxer) and parses both stderrs.
func (d *IntroDetector) runDetect(ctx context.Context, filePath string, startSec, durationSec float64) (silences, blacks []floatRange, err error) {
	// Audio first: it decodes in seconds, so when the media is unreadable the
	// pass fails before paying the video decode.
	for _, args := range [][]string{
		silenceDetectArgs(filePath, startSec, durationSec),
		blackDetectArgs(filePath, startSec, durationSec),
	} {
		cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "intro.detect"}, d.binary, args...)
		if err != nil {
			return nil, nil, err
		}
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf
		// ffmpeg exits non-zero for -f null, so the exit code alone says nothing
		// and stderr is parsed regardless. A process killed by a SIGNAL is a
		// different event: swallowing it reported an OOM kill as "nothing found",
		// which sent the media back through the sweep to kill the host again.
		runErr := finish(d.runPausable(cmd), stderrBuf.Bytes())
		if errors.Is(runErr, procfork.ErrMemoryCapExceeded) {
			return nil, nil, fmt.Errorf("%w: %s", runErr, filePath)
		}
		if killedBySignal(ctx, runErr) {
			return nil, nil, fmt.Errorf("%w: %s: %w", domainlibrary.ErrDetectorKilled, filePath, runErr)
		}
		s, b := parseDetections(stderrBuf.String())
		silences = append(silences, s...)
		blacks = append(blacks, b...)
	}
	return silences, blacks, nil
}

type floatRange struct{ start, end float64 }

// parseDetections extracts silence/black ranges from FFmpeg stderr text.
// FFmpeg silencedetect produces two lines: one with silence_start, one with
// silence_end + silence_duration. blackdetect produces ONE line with
// black_start, black_end, and black_duration all together.
func parseDetections(output string) (silences, blacks []floatRange) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var silStart float64
	var silStartSeen bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "silence_start:") {
			silStart = parseAfterColon(line, "silence_start:")
			silStartSeen = true
		}
		if strings.Contains(line, "silence_end:") && silStartSeen {
			end := parseAfterPipe(line, "silence_end:")
			if end > silStart {
				silences = append(silences, floatRange{silStart, end})
			}
			silStartSeen = false
		}
		if strings.Contains(line, "black_start:") {
			bs := parseAfterColon(line, "black_start:")
			be := parseAfterColon(line, "black_end:")
			if be > bs {
				blacks = append(blacks, floatRange{bs, be})
			} else {
				// black_end not on same line; store open range.
				blacks = append(blacks, floatRange{start: bs})
			}
		} else if strings.Contains(line, "black_end:") {
			be := parseAfterColon(line, "black_end:")
			if len(blacks) > 0 && blacks[len(blacks)-1].end == 0 {
				blacks[len(blacks)-1].end = be
			}
		}
	}
	return silences, blacks
}

// parseAfterColon extracts the float64 value following the first occurrence of
// key in line (format "key VALUE").
func parseAfterColon(line, key string) float64 {
	_, after, ok := strings.Cut(line, key)
	if !ok {
		return 0
	}
	rest := strings.TrimSpace(after)
	// rest may be "VALUE | other_key: ..." - take first token
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

// parseAfterPipe extracts the silence_end value; FFmpeg outputs
// "silence_end: X | silence_duration: Y".
func parseAfterPipe(line, key string) float64 {
	_, after, ok := strings.Cut(line, key)
	if !ok {
		return 0
	}
	rest := strings.TrimSpace(after)
	// cut at " | "
	if pipeIdx := strings.Index(rest, "|"); pipeIdx >= 0 {
		rest = strings.TrimSpace(rest[:pipeIdx])
	}
	v, _ := strconv.ParseFloat(rest, 64)
	return v
}

// mergeTransitions finds the earliest overlapping silence+black transition within
// [windowStart, windowEnd] and returns an approximate (start, end) pair.
// Falls back to any silence-only transition if no overlap exists.
func mergeTransitions(silences, blacks []floatRange, windowStart, windowEnd float64) (start, end float64, found bool) {
	overlap := func(a, b floatRange) (float64, float64, bool) {
		s := math.Max(a.start, b.start)
		e := math.Min(a.end, b.end)
		if e > s {
			return s, e, true
		}
		return 0, 0, false
	}
	inWindow := func(r floatRange) bool {
		return r.end > windowStart && r.start < windowEnd
	}

	for _, s := range silences {
		if !inWindow(s) {
			continue
		}
		for _, b := range blacks {
			if !inWindow(b) {
				continue
			}
			if os, oe, ok := overlap(s, b); ok {
				return os, oe, true
			}
		}
	}
	// fallback: first silence in window
	for _, s := range silences {
		if inWindow(s) {
			return s.start, s.end, true
		}
	}
	return 0, 0, false
}

var (
	reBlackStart    = regexp.MustCompile(`black_start:(\d+\.?\d*)`)
	reBlackDuration = regexp.MustCompile(`black_duration:(\d+\.?\d*)`)
)

// DetectEndCredits scans the last portion of a video file for black regions
// indicating end credits. Returns (startMs, endMs, true, nil) when a credits
// roll is detected in the window creditsScanWindowSec sizes (the last 20% of
// the file, capped at 10 minutes AND at the largest lead the plausibility bound
// accepts). Because the window is bounded, any statistic of the form "no
// credits before 80% of the file" is tautological: judge a credits marker by
// its LEAD (distance to the end of the file), never by its position percentage.
// Returns (0, 0, false, nil) when nothing useful is found - caller skips.
func (d *IntroDetector) DetectEndCredits(ctx context.Context, filePath string, durationSec float64) (startMs, endMs int64, found bool, err error) {
	return d.detectEndCreditsInWindow(ctx, filePath, durationSec, creditsScanWindowSec(durationSec))
}

// DetectEndCreditsWithin is DetectEndCredits with the decode held to the last
// maxLeadSec of the file: the season credits prior's re-scan of an outlier,
// where a black outside the season's stretch is the same scene fade again.
// 0 or less keeps the default window.
func (d *IntroDetector) DetectEndCreditsWithin(ctx context.Context, filePath string, durationSec, maxLeadSec float64) (startMs, endMs int64, found bool, err error) {
	return d.detectEndCreditsInWindow(ctx, filePath, durationSec, creditsWindowWithin(durationSec, maxLeadSec))
}

// creditsWindowWithin narrows the default window, never widens it.
func creditsWindowWithin(durationSec, maxLeadSec float64) float64 {
	window := creditsScanWindowSec(durationSec)
	if maxLeadSec > 0 && maxLeadSec < window {
		return maxLeadSec
	}
	return window
}

func (d *IntroDetector) detectEndCreditsInWindow(ctx context.Context, filePath string, durationSec, windowSec float64) (startMs, endMs int64, found bool, err error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.DetectEndCredits")
	defer span.End()
	if durationSec < 60 {
		return 0, 0, false, nil
	}
	startSec := math.Max(0, durationSec-windowSec)
	args := endCreditsArgs(filePath, startSec, windowSec)
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "intro.end_credits"}, d.binary, args...)
	if err != nil {
		return 0, 0, false, err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	// Same reasoning as runDetect: the -f null exit code is noise, a signal is not.
	runErr := finish(d.runPausable(cmd), out.Bytes())
	if errors.Is(runErr, procfork.ErrMemoryCapExceeded) {
		return 0, 0, false, fmt.Errorf("%w: %s", runErr, filePath)
	}
	if killedBySignal(ctx, runErr) {
		return 0, 0, false, fmt.Errorf("%w: %s: %w", domainlibrary.ErrDetectorKilled, filePath, runErr)
	}

	creditsStartMs, creditsEndMs, ok := parseEndCredits(out.String(), startSec, durationSec)
	if !ok {
		return 0, 0, false, nil
	}
	return creditsStartMs, creditsEndMs, true, nil
}

// creditsScanWindowSec returns how many seconds at the end of the file the
// blackdetect decode covers: the historic window, clamped to the largest lead
// parseEndCredits can still ACCEPT.
//
// Past that bound every decoded frame is spent building a cluster the same
// bound then refuses, so the clamp costs no detection: an accepted cluster
// starts at a lead <= the bound, hence entirely inside the clamped window, and
// dropping the regions before it can only shorten the trailing cluster from the
// front - which lowers its lead. It even gains detections, because an early
// fade sitting between the two edges used to be aggregated into the cluster and
// drag its first member past the bound, sinking the whole thing.
//
// The clamp is one-way on purpose. Letting the window FOLLOW the bound would
// widen the decode past the historic cap on long runtimes (+56% on a 2h10
// film), and films are where credits detection already finds almost nothing:
// paying more decode there would buy a hope, not a measured gain, while opening
// the 600-936s stretch to fades the bound would no longer refuse.
func creditsScanWindowSec(durationSec float64) float64 {
	legacy := math.Min(creditsScanCapSec, durationSec*creditsScanFraction)
	leadMaxSec := float64(domainlibrary.MaxPlausibleCreditsLeadMs(
		int64(durationSec*1000), domainlibrary.SuspiciousCreditsLeadMs)) / 1000
	return math.Min(legacy, leadMaxSec)
}

// Credits-scan window tuning for DetectEndCredits.
const (
	// creditsScanFraction: the tail of the file the decode covers. The code has
	// always used 20%, the comment used to say 10%.
	creditsScanFraction = 0.2
	// creditsScanCapSec caps the decode on long runtimes.
	creditsScanCapSec = 600.0
)

// Credits-selection tuning for parseEndCredits.
const (
	// minCreditsBlackSec: black regions shorter than this are ignored (frame
	// drops, flash cuts).
	minCreditsBlackSec = 1.0
	// creditsClusterGapSec: black regions closer than this belong to the same
	// cluster. Credits produce a trailing cluster (fade-in, card gaps, post-
	// credit black); an isolated mid-scene fade does not.
	creditsClusterGapSec = 120.0
	// creditsSingleRegionMaxLeadSec: a cluster of ONE region is only trusted
	// when it sits this close to the end of the file. An isolated black fade
	// minutes before the end is a scene transition, not the credits - taking
	// it produced "skip credits" prompts >10 minutes early.
	creditsSingleRegionMaxLeadSec = 300.0
	// creditsClusterMinBlackSec: alternatively, a single region carrying this
	// much black IS trusted farther out (a long fade-to-black credits opening).
	creditsClusterMinBlackSec = 5.0
)

// parseEndCredits locates the start of the credits roll in the blackdetect
// output produced for the window starting at startSec (offsets are relative to
// -ss startSec; returned ms are file-absolute).
//
// The credits are the TRAILING cluster of black regions - never simply the
// first black in the window. A lone black fade far from the end (a scene
// transition inside the scan window) previously became "credits from here to
// the end", surfacing a skip prompt many minutes early; it is now rejected
// unless it is either close to the end or carries a long stretch of black.
func parseEndCredits(output string, startSec, durationSec float64) (startMs, endMs int64, found bool) {
	type blackRegion struct{ startSec, durSec float64 }
	var regions []blackRegion
	for line := range strings.SplitSeq(output, "\n") {
		startMatches := reBlackStart.FindStringSubmatch(line)
		durMatches := reBlackDuration.FindStringSubmatch(line)
		if len(startMatches) < 2 || len(durMatches) < 2 {
			continue
		}
		offset, err1 := strconv.ParseFloat(startMatches[1], 64)
		dur, err2 := strconv.ParseFloat(durMatches[1], 64)
		if err1 != nil || err2 != nil || dur < minCreditsBlackSec {
			continue
		}
		regions = append(regions, blackRegion{startSec: startSec + offset, durSec: dur})
	}
	if len(regions) == 0 {
		return 0, 0, false
	}

	// Trailing cluster: walk backwards from the last region while gaps stay
	// within creditsClusterGapSec.
	first := len(regions) - 1
	for first > 0 {
		prev := regions[first-1]
		if regions[first].startSec-(prev.startSec+prev.durSec) > creditsClusterGapSec {
			break
		}
		first--
	}
	cluster := regions[first:]

	var clusterBlackSec float64
	for _, r := range cluster {
		clusterBlackSec += r.durSec
	}
	lead := durationSec - cluster[0].startSec
	// The plausibility bound applies to EVERY trust path: a two-region cluster
	// and a long lone black were both unbounded, and either could plant the
	// credits anywhere in the scanned window - up to 600s before the end, the
	// value that keeps coming back in the prod sample. Same function as the
	// serving gate and the suspect-credits repair: one answer to "too early?".
	if domainlibrary.CreditsLeadImplausible(int64(lead*1000), int64(durationSec*1000), domainlibrary.SuspiciousCreditsLeadMs) {
		return 0, 0, false
	}
	trusted := len(cluster) >= 2 ||
		clusterBlackSec >= creditsClusterMinBlackSec ||
		lead <= creditsSingleRegionMaxLeadSec
	if !trusted {
		return 0, 0, false
	}
	return int64(cluster[0].startSec * 1000), int64(durationSec * 1000), true
}

// Leading-logo scan tuning for DetectLeadingLogoBlock.
const (
	// leadingLogoScanWindowSec bounds the head decode. The longest labelled
	// distributor block in the gold ground-truth set is 172s; 240s leaves
	// margin without opening the
	// door to the film's first real scene fade.
	leadingLogoScanWindowSec = 240.0
)

// Leading-logo selection tuning for parseLeadingLogoBlock.
const (
	// leadingLogoMinBlackSec: black regions shorter than this are ignored.
	// The blackdetect filter itself already refuses to report anything
	// shorter than its d=0.5 parameter, so this is set equal to it rather
	// than to minCreditsBlackSec's stricter 1.0s: distributor-logo fades are
	// quick cuts, and raising the floor above the filter's own d would start
	// dropping real logo transitions the filter already accepted.
	leadingLogoMinBlackSec = 0.5
	// leadingLogoFirstRegionMaxStartSec: the cluster's first region must
	// begin this close to t=0. A film that opens on picture (no black) has
	// no logo block to find - every labelled example in the ground-truth
	// set starts on black within the first couple of seconds.
	leadingLogoFirstRegionMaxStartSec = 5.0
	// leadingLogoClusterGapSec: black regions closer than this belong to the
	// same leading cluster. Individual distributor idents run ~5-10s each,
	// separated by a short fade; 25s comfortably spans that gap while still
	// cutting the cluster off well before the film's first real scene
	// transition.
	leadingLogoClusterGapSec = 25.0
	// leadingLogoSingleRegionMinBlackSec: a cluster of a single region is
	// only trusted when it carries this much black - a lone brief fade-in is
	// a scene opening, not a logo block. A held distributor card usually
	// holds several seconds of black; a scene fade-in is typically under 2s.
	leadingLogoSingleRegionMinBlackSec = 5.0
	// leadingLogoMaxPlausibleSec absolutely caps the result: past this a
	// "logo block" is no longer plausible as one, it is scene content.
	leadingLogoMaxPlausibleSec = 300.0
	// leadingLogoMaxDurationFraction additionally caps the result relative
	// to the film's own runtime, so a short feature can't be handed several
	// minutes of "logos". The ground-truth set's worst observed ratio is
	// ~2.8% (172s on a ~103-minute film); 10% leaves a >3x margin.
	leadingLogoMaxDurationFraction = 0.10
)

// DetectLeadingLogoBlock scans the HEAD of a video file for a cluster of
// black regions indicating a distributor-logo block preceding a film's real
// content. Unlike DetectEndCredits, this is a FILM-only signal: series label
// the same pattern (a distributor ident on black at 0:00) explicitly as NOT
// an intro, since it is common to every episode. The caller is responsible
// for gating this pass on the media type.
//
// Returns (0, endMs, true, nil) when a leading cluster is found - the start
// is always 0 by construction, matching every ground-truth label for this
// pattern. Returns (0, 0, false, nil) when nothing useful is found.
func (d *IntroDetector) DetectLeadingLogoBlock(ctx context.Context, filePath string, durationSec float64) (startMs, endMs int64, found bool, err error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.DetectLeadingLogoBlock")
	defer span.End()
	if durationSec <= 0 {
		return 0, 0, false, nil
	}
	windowSec := math.Min(leadingLogoScanWindowSec, durationSec)

	args := []string{
		"-nostdin",
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", "0",
		"-t", strconv.FormatFloat(windowSec, 'f', 3, 64),
		"-i", filePath,
		"-vf", "blackdetect=d=0.5:pix_th=0.10",
		"-an",
		"-vsync", "vfr",
		"-f", "null",
		"-",
	}
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "intro.leading_logo"}, d.binary, args...)
	if err != nil {
		return 0, 0, false, err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	// ffmpeg exits non-zero for -f null; the combined output is parsed regardless.
	if runErr := finish(d.runPausable(cmd), out.Bytes()); errors.Is(runErr, procfork.ErrMemoryCapExceeded) {
		return 0, 0, false, fmt.Errorf("%w: %s", runErr, filePath)
	}

	logoStartMs, logoEndMs, ok := parseLeadingLogoBlock(out.String(), windowSec, durationSec)
	if !ok {
		return 0, 0, false, nil
	}
	return logoStartMs, logoEndMs, true, nil
}

// parseLeadingLogoBlock locates the leading cluster of black regions in the
// blackdetect output produced for the head of the file (offsets relative to
// t=0, since the scan always starts at -ss 0). windowSec is the length of the
// decode; durationSec is the film's total runtime.
//
// The result is the LEADING cluster - the first region has to sit near t=0
// (a film opening on picture has no logo block), and the cluster only
// extends forward while consecutive regions stay close enough together to be
// the same run of distributor idents.
func parseLeadingLogoBlock(output string, windowSec, durationSec float64) (startMs, endMs int64, found bool) {
	type blackRegion struct{ startSec, durSec float64 }
	var regions []blackRegion
	for line := range strings.SplitSeq(output, "\n") {
		startMatches := reBlackStart.FindStringSubmatch(line)
		durMatches := reBlackDuration.FindStringSubmatch(line)
		if len(startMatches) < 2 || len(durMatches) < 2 {
			continue
		}
		offset, err1 := strconv.ParseFloat(startMatches[1], 64)
		dur, err2 := strconv.ParseFloat(durMatches[1], 64)
		if err1 != nil || err2 != nil || dur < leadingLogoMinBlackSec {
			continue
		}
		regions = append(regions, blackRegion{startSec: offset, durSec: dur})
	}
	if len(regions) == 0 {
		return 0, 0, false
	}

	if regions[0].startSec > leadingLogoFirstRegionMaxStartSec {
		return 0, 0, false
	}

	// Leading cluster: walk forward from the first region while gaps stay
	// within leadingLogoClusterGapSec.
	last := 0
	for last+1 < len(regions) {
		prev := regions[last]
		next := regions[last+1]
		if next.startSec-(prev.startSec+prev.durSec) > leadingLogoClusterGapSec {
			break
		}
		last++
	}
	cluster := regions[:last+1]

	var clusterBlackSec float64
	for _, r := range cluster {
		clusterBlackSec += r.durSec
	}
	trusted := len(cluster) >= 2 || clusterBlackSec >= leadingLogoSingleRegionMinBlackSec
	if !trusted {
		return 0, 0, false
	}

	end := cluster[len(cluster)-1].startSec + cluster[len(cluster)-1].durSec
	// Plausibility cap: bounded absolutely, relative to the film's own
	// runtime, and never past the scanned window.
	cap := min(leadingLogoMaxPlausibleSec, durationSec*leadingLogoMaxDurationFraction, windowSec)
	if end > cap {
		return 0, 0, false
	}
	// The serving gate's own floor, read from the domain rather than mirrored
	// here: a candidate under it would never be served, so storing it is waste.
	if end*1000 < float64(domainlibrary.MinDetectedIntroMs) {
		return 0, 0, false
	}
	return 0, int64(end * 1000), true
}

// DetectStudioLogo searches the first ~90 seconds of trickplay frame hashes for
// a match against the embedded studio logo catalog. intervalSec is the number of
// seconds between consecutive trickplay frames (e.g. 15 for default trickplay).
// Returns (startMs, endMs, true) when a match clears the minMatches threshold.
func (d *IntroDetector) DetectStudioLogo(hashes []uint64, intervalSec int) (startMs, endMs int64, found bool) {
	return detectStudioLogoIn(hashes, intervalSec, loadedStudioLogos, 5)
}

// AudioRMSProfile decodes a slice of the file [startSec, startSec+durationSec)
// to PCM mono 8 kHz s16le and returns RMS energy averaged over fixed-length bins
// (default 250 ms = 2000 samples). Used as a low-cost audio fingerprint for
// cross-episode intro confirmation: the same musical theme will yield highly
// correlated RMS envelopes across episodes; a coincidental scene transition
// will not.
//
// Returns nil + error on decode failure. Returns an empty slice without error
// if the segment is shorter than one bin.
func (d *IntroDetector) AudioRMSProfile(ctx context.Context, filePath string, startSec, durationSec float64, binMs int) ([]float64, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.AudioRMSProfile")
	defer span.End()
	if binMs <= 0 {
		binMs = 250
	}
	const sampleRate = 8000
	samplesPerBin := sampleRate * binMs / 1000

	args := []string{
		"-nostdin",
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-i", filePath,
		"-vn",
		"-ar", strconv.Itoa(sampleRate),
		"-ac", "1",
		"-f", "s16le",
		"pipe:1",
	}
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "intro.audio_envelope"}, d.binary, args...)
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := finish(d.runPausable(cmd), nil); err != nil {
		// FFmpeg may exit non-zero on stream end; tolerate when we got bytes.
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("ffmpeg pcm decode: %w", err)
		}
	}

	raw := stdout.Bytes()
	totalSamples := len(raw) / 2
	if totalSamples < samplesPerBin {
		return []float64{}, nil
	}
	bins := totalSamples / samplesPerBin
	out := make([]float64, bins)
	for b := range bins {
		var acc float64
		base := b * samplesPerBin * 2
		for i := 0; i < samplesPerBin; i++ {
			lo := raw[base+i*2]
			hi := raw[base+i*2+1]
			s := int16(uint16(lo) | uint16(hi)<<8)
			f := float64(s) / 32768.0
			acc += f * f
		}
		out[b] = math.Sqrt(acc / float64(samplesPerBin))
	}
	return out, nil
}

// SceneCutTimestamps returns the timestamps (ms, ascending, absolute) of every
// frame whose scene-change score exceeds threshold within [startSec, +duration].
// Frame-accurate content cuts - the post-intro content resumes at one of these.
// Used as a witness/snap target for the intro END (always available from video,
// even when the show has no subtitles). threshold ~0.3-0.4 isolates real cuts.
func (d *IntroDetector) SceneCutTimestamps(ctx context.Context, filePath string, startSec, durationSec, threshold float64) ([]int64, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.SceneCutTimestamps")
	defer span.End()
	if filePath == "" || durationSec <= 0 {
		return nil, nil
	}
	args := []string{
		"-nostdin",
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-i", filePath,
		"-an",
		"-vf", fmt.Sprintf("select='gt(scene,%.3f)',showinfo", threshold),
		"-f", "null",
		"-",
	}
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "intro.scene_cuts"}, d.binary, args...)
	if err != nil {
		return nil, err
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	// ffmpeg exits non-zero for -f null; stderr is parsed regardless.
	if runErr := finish(d.runPausable(cmd), stderrBuf.Bytes()); errors.Is(runErr, procfork.ErrMemoryCapExceeded) {
		return nil, fmt.Errorf("%w: %s", runErr, filePath)
	}

	// -ss before -i resets output pts to 0 at the segment start, so showinfo's
	// pts_time is relative to startSec; add it back for an absolute timestamp.
	var cuts []int64
	sc := bufio.NewScanner(&stderrBuf)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, "pts_time:") {
			continue
		}
		pt := parseAfterColon(line, "pts_time:")
		if pt <= 0 {
			continue
		}
		cuts = append(cuts, int64((startSec+pt)*1000))
	}
	slices.Sort(cuts)
	return cuts, nil
}

// SubtitleCueStarts returns the start timestamps (ms, ascending) of every cue
// in the file's first subtitle stream, read from packet pts_time via ffprobe.
// Cue TIMING is language-independent, so the first stream suffices regardless of
// language or codec (text or bitmap). Returns an empty slice with no error when
// the file has no usable subtitle stream - callers then treat it as a no-op.
func (d *IntroDetector) SubtitleCueStarts(ctx context.Context, filePath string) ([]int64, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.SubtitleCueStarts")
	defer span.End()
	if filePath == "" {
		return nil, nil
	}
	if d.subtitleCues != nil {
		if cues, ok := d.subtitleCues(ctx, filePath); ok {
			span.AddEvent("subtitle-cues-from-index")
			return cues, nil
		}
	}
	args := []string{
		"-v", "error",
		"-select_streams", "s:0",
		"-show_entries", "packet=pts_time",
		"-of", "csv=p=0",
		filePath,
	}
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "intro.subtitle_cues"}, d.probeBinary, args...)
	if err != nil {
		// No subtitle stream / probe failure: a no-op, not an error.
		return nil, nil
	}
	var cues []int64
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line == "N/A" {
			continue
		}
		sec, err := strconv.ParseFloat(line, 64)
		if err != nil || sec < 0 {
			continue
		}
		cues = append(cues, int64(sec*1000))
	}
	// Packet order usually follows pts but isn't guaranteed; sort to be safe.
	slices.Sort(cues)
	return cues, nil
}

// AudioBandedFingerprint decodes the segment to PCM 8kHz mono and produces
// a per-bin spectral fingerprint: for each fixed-length bin (binMs), it runs
// the Goertzel algorithm at 6 log-spaced frequencies (~120, 250, 500, 1000,
// 2000, 3500 Hz) and emits 6 log-magnitudes per bin.
//
// Returned shape: []float64 of length numBins*6, ordered
// [bin0_b0, bin0_b1,...,bin0_b5, bin1_b0,...].
//
// Compared to plain RMS envelope (AudioRMSProfile), this captures *which*
// frequencies dominate over time - robust against different audio mixes
// (FRENCH vs MULTI dub) carrying the same underlying music: the dialogue
// channel changes RMS but the band ratios still reflect the music's spectral
// structure.
func (d *IntroDetector) AudioBandedFingerprint(ctx context.Context, filePath string, startSec, durationSec float64, binMs int) ([]float64, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.AudioBandedFingerprint")
	defer span.End()
	if binMs <= 0 {
		binMs = 250
	}
	const sampleRate = 8000
	samplesPerBin := sampleRate * binMs / 1000

	args := []string{
		"-nostdin",
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-i", filePath,
		"-vn",
		"-ar", strconv.Itoa(sampleRate),
		"-ac", "1",
		"-f", "s16le",
		"pipe:1",
	}
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "intro.audio_bands"}, d.binary, args...)
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := finish(d.runPausable(cmd), nil); err != nil {
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("ffmpeg pcm decode: %w", err)
		}
	}

	raw := stdout.Bytes()
	totalSamples := len(raw) / 2
	if totalSamples < samplesPerBin {
		return []float64{}, nil
	}
	bins := totalSamples / samplesPerBin

	// 12 log-spaced target frequencies (Hz), within 0..Nyquist (4000Hz).
	freqs := []float64{80, 150, 250, 400, 600, 900, 1300, 1800, 2400, 3000, 3500, 3800}
	// Goertzel coefficients for each freq, given samplesPerBin and sampleRate.
	cosw := make([]float64, len(freqs))
	for i, f := range freqs {
		w := 2 * math.Pi * f / float64(sampleRate)
		cosw[i] = math.Cos(w)
	}

	// Decode samples into a float buffer once.
	samples := make([]float64, totalSamples)
	for i := 0; i < totalSamples; i++ {
		lo := raw[i*2]
		hi := raw[i*2+1]
		s := int16(uint16(lo) | uint16(hi)<<8)
		samples[i] = float64(s) / 32768.0
	}

	nb := len(freqs)
	out := make([]float64, bins*nb)
	for b := 0; b < bins; b++ {
		bin := samples[b*samplesPerBin : (b+1)*samplesPerBin]
		// Per-bin sum used for L1 normalization across bands. Without this,
		// the loudness component dominates Pearson and a quiet vs loud rendering
		// of the same intro music will not correlate.
		var sum float64
		mags := out[b*nb : (b+1)*nb]
		for k, c := range cosw {
			var q1, q2 float64
			twoCos := 2 * c
			for _, s := range bin {
				q0 := twoCos*q1 - q2 + s
				q2 = q1
				q1 = q0
			}
			mag := math.Sqrt(q1*q1 + q2*q2 - twoCos*q1*q2)
			lm := math.Log1p(mag)
			mags[k] = lm
			sum += lm
		}
		if sum > 0 {
			for k := range mags {
				mags[k] /= sum
			}
		}
	}
	return out, nil
}

// PearsonCorrelation computes the Pearson correlation coefficient between two
// equal-length series. Returns 0 for empty/zero-variance inputs to avoid NaN.
// Range: [-1, 1]. Higher means more similar shape (invariant to mean and scale).
func PearsonCorrelation(a, b []float64) float64 {
	n := len(a)
	if n == 0 || n != len(b) {
		return 0
	}
	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)
	var num, dA, dB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		num += da * db
		dA += da * da
		dB += db * db
	}
	if dA == 0 || dB == 0 {
		return 0
	}
	return num / math.Sqrt(dA*dB)
}

// SlidingBestPearsonBanded slides a multi-channel banded fingerprint
// (vectorSize floats per time bin, e.g. 6 bands) target window over
// reference one *time bin* at a time (vectorSize floats) and computes
// Pearson on the full multi-band concatenation per offset. Returns the
// best correlation and the offset measured in time bins.
//
// This is the core of cross-episode audio confirmation: target audio
// profile is the candidate intro window from one episode; reference is a
// wider candidate region from a sibling episode. A clean peak indicates
// the two episodes share the same audio signal - a real recurring intro
// - even if the absolute timestamp drifts across episodes.
func SlidingBestPearsonBanded(target, reference []float64, vectorSize int) (bestCorr float64, bestOffsetBins int) {
	if vectorSize <= 0 {
		return 0, 0
	}
	if len(target)%vectorSize != 0 || len(reference)%vectorSize != 0 {
		return 0, 0
	}
	if len(reference) < len(target) || len(target) == 0 {
		return 0, 0
	}
	maxOffBins := (len(reference) - len(target)) / vectorSize
	bestCorr = -1
	for offBins := 0; offBins <= maxOffBins; offBins++ {
		offFloats := offBins * vectorSize
		c := PearsonCorrelation(target, reference[offFloats:offFloats+len(target)])
		if c > bestCorr {
			bestCorr = c
			bestOffsetBins = offBins
		}
	}
	return bestCorr, bestOffsetBins
}

// EdgeThumbWidth and EdgeThumbHeight size the grey thumbnails GrayThumbnails decodes:
// small enough that a 20s window is a few KB, large enough to tell a title card from
// a scene.
const (
	EdgeThumbWidth  = 32
	EdgeThumbHeight = 18
)

// GrayThumbnails decodes [startSec, startSec+durationSec] into EdgeThumbWidth x
// EdgeThumbHeight 8-bit grey frames at fps frames per second, in order. It feeds the
// visual widening of an audio-confirmed intro, which compares these frames across
// episodes at their audio alignment.
func (d *IntroDetector) GrayThumbnails(ctx context.Context, filePath string, startSec, durationSec, fps float64) ([][]byte, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.intro.GrayThumbnails")
	defer span.End()
	if filePath == "" || durationSec <= 0 || fps <= 0 {
		return nil, nil
	}
	args := []string{
		"-nostdin", "-v", "error",
		"-threads", strconv.Itoa(introFFmpegThreads()),
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-i", filePath,
		"-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("fps=%g,scale=%d:%d,format=gray", fps, EdgeThumbWidth, EdgeThumbHeight),
		"-f", "rawvideo", "-",
	}
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Scan, Site: "intro.edge_frames"}, d.binary, args...)
	if err != nil {
		return nil, fmt.Errorf("gray thumbnails %s: %w", filePath, err)
	}
	const frameSize = EdgeThumbWidth * EdgeThumbHeight
	frames := make([][]byte, 0, len(out)/frameSize)
	for i := 0; i+frameSize <= len(out); i += frameSize {
		frames = append(frames, out[i:i+frameSize])
	}
	return frames, nil
}
