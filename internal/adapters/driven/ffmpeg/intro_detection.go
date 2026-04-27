package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// IntroDetector performs pHash-based intro detection and FFmpeg audio/black passes.
type IntroDetector struct {
	binary      string
	probeBinary string
}

func NewIntroDetector(binary, probeBinary string) *IntroDetector {
	return &IntroDetector{binary: binary, probeBinary: probeBinary}
}

// ExtractTileHashes opens a trickplay sprite file and returns one aHash per tile.
// Tiles are laid out in a tileWidth×tileHeight grid; each tile is tilePixelWidth px wide
// and tilePixelHeight px tall.
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
			hashes = append(hashes, aHash(img, x0, y0, x1, y1))
		}
	}
	return hashes, nil
}

// aHash computes a 64-bit average hash of the image region [x0,y0,x1,y1].
// The region is scaled to 8×8, converted to greyscale, and bits set where
// pixel luminance >= mean.
func aHash(img image.Image, x0, y0, x1, y1 int) uint64 {
	const size = 8
	dst := image.NewGray(image.Rect(0, 0, size, size))
	src := img

	// If the region is a sub-image crop, wrap in a sub-rectangle first.
	srcBounds := image.Rect(x0, y0, x1, y1)
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := src.(subImager); ok {
		src = si.SubImage(srcBounds)
	}

	draw.BiLinear.Scale(dst, dst.Bounds(), src, srcBounds, draw.Src, nil)

	var sum uint64
	for i := range size * size {
		sum += uint64(dst.Pix[i])
	}
	mean := sum / (size * size)

	var h uint64
	for i := range size * size {
		if uint64(dst.Pix[i]) > mean {
			h |= 1 << uint(i)
		}
	}
	return h
}

// PrecisionPass runs FFmpeg silencedetect+blackdetect on [startSec-padSec, endSec+padSec].
// Returns refined (startMs, endMs). Falls back to (startMs_in, endMs_in) when FFmpeg
// finds nothing useful.
func (d *IntroDetector) PrecisionPass(ctx context.Context, filePath string, startSec, endSec, padSec float64) (startMs, endMs int64, err error) {
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

// runDetect executes FFmpeg with silencedetect and blackdetect filters and parses
// the stderr output.
func (d *IntroDetector) runDetect(ctx context.Context, filePath string, startSec, durationSec float64) (silences, blacks []floatRange, err error) {
	args := []string{
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-i", filePath,
		"-vf", "blackdetect=d=0.1:pix_th=0.10",
		"-af", "silencedetect=n=-50dB:d=0.5",
		"-f", "null",
		"-",
	}
	cmd := exec.CommandContext(ctx, d.binary, args...)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	// FFmpeg always exits non-zero for -f null; ignore run error but keep stderr.
	_ = cmd.Run()

	silences, blacks = parseDetections(stderrBuf.String())
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
	idx := strings.Index(line, key)
	if idx < 0 {
		return 0
	}
	rest := strings.TrimSpace(line[idx+len(key):])
	// rest may be "VALUE | other_key: ..." — take first token
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
	idx := strings.Index(line, key)
	if idx < 0 {
		return 0
	}
	rest := strings.TrimSpace(line[idx+len(key):])
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
// roll is detected (sustained black region in the last 10% / max 10min window).
// Returns (0, 0, false, nil) when nothing useful is found — caller skips.
func (d *IntroDetector) DetectEndCredits(ctx context.Context, filePath string, durationSec float64) (startMs, endMs int64, found bool, err error) {
	if durationSec < 60 {
		return 0, 0, false, nil
	}
	startSec := math.Max(0, durationSec-math.Min(600, durationSec*0.2))

	args := []string{
		"nice", "-n", "19",
		d.binary,
		"-nostdin",
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-i", filePath,
		"-vf", "blackdetect=d=0.5:pix_th=0.10",
		"-an",
		"-vsync", "vfr",
		"-f", "null",
		"-",
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	out, _ := cmd.CombinedOutput()

	creditsStartMs, creditsEndMs, ok := parseEndCredits(string(out), startSec, durationSec)
	if !ok {
		return 0, 0, false, nil
	}
	return creditsStartMs, creditsEndMs, true, nil
}

// parseEndCredits finds the first black region with duration >= 1.0s in the
// FFmpeg output produced for the window starting at startSec. Offsets in the
// output are relative to -ss startSec and are converted back to file-absolute ms.
func parseEndCredits(output string, startSec, durationSec float64) (startMs, endMs int64, found bool) {
	for _, line := range strings.Split(output, "\n") {
		startMatches := reBlackStart.FindStringSubmatch(line)
		durMatches := reBlackDuration.FindStringSubmatch(line)
		if len(startMatches) < 2 || len(durMatches) < 2 {
			continue
		}
		offset, err1 := strconv.ParseFloat(startMatches[1], 64)
		dur, err2 := strconv.ParseFloat(durMatches[1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if dur >= 1.0 {
			return int64((startSec + offset) * 1000), int64(durationSec * 1000), true
		}
	}
	return 0, 0, false
}

// DetectStudioLogo searches the first ~90 seconds of trickplay frame hashes for
// a match against the embedded studio logo catalog. intervalSec is the number of
// seconds between consecutive trickplay frames (e.g. 15 for default trickplay).
// Returns (startMs, endMs, true) when a match clears the minMatches threshold.
func (d *IntroDetector) DetectStudioLogo(hashes []uint64, intervalSec int) (startMs, endMs int64, found bool) {
	return detectStudioLogoIn(hashes, intervalSec, loadedStudioLogos, 5)
}

// grayscaleLuminance converts a color to uint8 luminance using BT.601 coefficients.
// Kept here for potential future use; currently draw.BiLinear produces Gray pixels.
func grayscaleLuminance(c color.Color) uint8 {
	r, g, b, _ := c.RGBA()
	lum := (19595*r + 38470*g + 7471*b + 1<<15) >> 24
	return uint8(lum)
}

// Suppress "declared but not used" for grayscaleLuminance when it has no callers.
var _ = grayscaleLuminance
