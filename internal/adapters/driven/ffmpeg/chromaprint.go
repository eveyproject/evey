package ffmpeg

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/bits"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Chromaprinter is the port for fpcalc.
//
// Chromaprint fingerprints are fixed-rate (~8 frames/second, each frame
// = uint32). Comparing two fingerprints by sliding XOR + popcount is the
// standard "intro detection by audio match" approach used by Jellyfin
// Intro Skipper, MusicBrainz, and Acoustid. Robust to bitrate, codec,
// language track, and minor mix differences.
type Chromaprinter struct {
	binary string
}

func NewChromaprinter(binary string) *Chromaprinter {
	if binary == "" {
		binary = "fpcalc"
	}
	return &Chromaprinter{binary: binary}
}

// Available returns nil when the fpcalc binary is callable. Use it at
// startup to decide whether to wire the Chromaprint pass into the
// detection pipeline; absence is a soft failure (fallback to pHash + audio
// banded confirmation).
func (c *Chromaprinter) Available(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binary, "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fpcalc -version: %w", err)
	}
	return nil
}

// Compute extracts a Chromaprint fingerprint from [startSec, startSec+lengthSec]
// of the file. Returns the raw uint32 frames (needed for inter-episode
// comparison; the compact base64 string returned by fpcalc by default is for
// AcoustID lookups and can't be diffed bitwise).
//
// fpcalc itself has no seek option — its -ts switch is for AcoustID timestamp
// metadata, not for skipping into the input — so we pipe the audio through
// ffmpeg with -ss/-t and let fpcalc read from stdin.
//
// Returns the underlying []uint32 (typed alias) so the app-layer port can
// declare its dependency without importing this adapter package.
func (c *Chromaprinter) Compute(ctx context.Context, filePath string, startSec, lengthSec float64) ([]uint32, error) {
	if filePath == "" {
		return nil, errors.New("empty file path")
	}
	if lengthSec <= 0 {
		lengthSec = 240 // default: scan first 4 minutes (covers the typical intro region)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	ffmpegArgs := []string{"-nostdin", "-hide_banner", "-loglevel", "error",
		"-threads", strconv.Itoa(introFFmpegThreads()),
	}
	if startSec > 0 {
		ffmpegArgs = append(ffmpegArgs, "-ss", strconv.FormatFloat(startSec, 'f', 3, 64))
	}
	ffmpegArgs = append(ffmpegArgs,
		"-i", filePath,
		"-t", strconv.FormatFloat(lengthSec, 'f', 3, 64),
		"-map", "0:a:0",
		"-vn",
		"-f", "wav",
		"-",
	)
	ffmpegCmd := exec.CommandContext(ctx, "ffmpeg", ffmpegArgs...)
	pipe, err := ffmpegCmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	ffmpegCmd.Stderr = nil
	if err := ffmpegCmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	fpCmd := exec.CommandContext(ctx, c.binary, "-raw", "-length", strconv.FormatFloat(lengthSec, 'f', 0, 64), "-")
	fpCmd.Stdin = pipe
	out, fpErr := fpCmd.Output()
	// Wait for ffmpeg to finish so we don't leak the process; ignore its exit
	// status because EOF on stdout is normal once fpcalc closed the pipe.
	_ = ffmpegCmd.Wait()
	if fpErr != nil {
		return nil, fmt.Errorf("fpcalc: %w", fpErr)
	}
	return parseFpcalcRawOutput(out)
}

// parseFpcalcRawOutput reads fpcalc -raw stdout. The format is two lines:
//
//	DURATION=...
//	FINGERPRINT=12345,67890,...,42
//
// (-raw replaces the default base64-encoded FINGERPRINT with a comma-
// separated list of int32 values that we can diff bitwise.)
func parseFpcalcRawOutput(buf []byte) ([]uint32, error) {
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "FINGERPRINT=") {
			continue
		}
		raw := strings.TrimPrefix(line, "FINGERPRINT=")
		parts := strings.Split(raw, ",")
		fp := make([]uint32, 0, len(parts))
		for _, p := range parts {
			n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse frame %q: %w", p, err)
			}
			fp = append(fp, uint32(n))
		}
		if len(fp) == 0 {
			return nil, errors.New("empty fingerprint")
		}
		return fp, nil
	}
	return nil, errors.New("FINGERPRINT line not found in fpcalc output")
}

// EncodeBase64 packs the raw frame slice as little-endian uint32 bytes and
// base64-encodes — convenient compact storage in the fingerprint cache.
func EncodeBase64Fingerprint(fp []uint32) string {
	if len(fp) == 0 {
		return ""
	}
	buf := make([]byte, 4*len(fp))
	for i, v := range fp {
		buf[i*4+0] = byte(v)
		buf[i*4+1] = byte(v >> 8)
		buf[i*4+2] = byte(v >> 16)
		buf[i*4+3] = byte(v >> 24)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// DecodeBase64Fingerprint reverses EncodeBase64Fingerprint.
func DecodeBase64Fingerprint(s string) ([]uint32, error) {
	if s == "" {
		return nil, nil
	}
	buf, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("buffer len %d not multiple of 4", len(buf))
	}
	fp := make([]uint32, len(buf)/4)
	for i := range fp {
		fp[i] = uint32(buf[i*4+0]) |
			uint32(buf[i*4+1])<<8 |
			uint32(buf[i*4+2])<<16 |
			uint32(buf[i*4+3])<<24
	}
	return fp, nil
}

// SlidingBestMatch slides target over reference and returns (bestOffsetFrames,
// bestSimilarity). bestSimilarity is in [0, 1] — 1 means identical, ~0.5 is
// noise. Common cutoff for "matched intro" is ≥ 0.85.
//
// Algorithm: at each candidate offset where target fully overlaps reference,
// compute Hamming distance per frame (popcount of XOR), sum, then convert
// to similarity = 1 - (sum / (32 * frames)).
//
// Both fingerprints must come from the same fpcalc version (frame layout
// is implementation-defined). Same-version-same-source guaranteed by the
// fingerprint cache: every cached fingerprint carries an algo_version.
func SlidingBestMatch(target, reference []uint32) (offsetFrames int, similarity float64) {
	if len(target) == 0 || len(reference) < len(target) {
		return 0, 0
	}
	maxBits := 32 * len(target)
	bestOffset, bestBits := 0, maxBits
	for offset := 0; offset <= len(reference)-len(target); offset++ {
		var diffBits int
		for i := range target {
			diffBits += bits.OnesCount32(target[i] ^ reference[offset+i])
		}
		if diffBits < bestBits {
			bestBits = diffBits
			bestOffset = offset
		}
	}
	sim := 1.0 - float64(bestBits)/float64(maxBits)
	return bestOffset, sim
}
