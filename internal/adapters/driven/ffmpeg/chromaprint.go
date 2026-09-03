package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

// Chromaprinter is the port for fpcalc.
//
// Chromaprint fingerprints are fixed-rate (~8 frames/second, each frame
// = uint32). Comparing two fingerprints by sliding XOR + popcount is the
// standard "intro detection by audio match" approach
// Intro Skipper, MusicBrainz, and Acoustid. Robust to bitrate, codec,
// language track, and minor mix differences.
type Chromaprinter struct {
	binary string
	// busy, when set and true, suspends the audio-decode ffmpeg while real work
	// runs (see pauseProcessWhileBusy). nil = run flat out.
	busy func() bool
}

// WithBusyChecker wires the predicate used to SIGSTOP the fingerprint's ffmpeg
// while a playback/transcode session or a recalibration is active. nil-safe.
func (c *Chromaprinter) WithBusyChecker(busy func() bool) *Chromaprinter {
	c.busy = busy
	return c
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
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.chromaprint.Available")
	defer span.End()
	if err := procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "chromaprint.version"}, c.binary, "-version"); err != nil {
		return fmt.Errorf("fpcalc -version: %w", err)
	}
	return nil
}

// Compute extracts a Chromaprint fingerprint from [startSec, startSec+lengthSec]
// of the file. Returns the raw uint32 frames (needed for inter-episode
// comparison; the compact base64 string returned by fpcalc by default is for
// AcoustID lookups and can't be diffed bitwise).
//
// fpcalc itself has no seek option - its -ts switch is for AcoustID timestamp
// metadata, not for skipping into the input - so we pipe the audio through
// ffmpeg with -ss/-t and let fpcalc read from stdin.
//
// Returns the underlying []uint32 (typed alias) so the app-layer port can
// declare its dependency without importing this adapter package.
func (c *Chromaprinter) Compute(ctx context.Context, filePath string, startSec, lengthSec float64) ([]uint32, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.chromaprint.Compute")
	defer span.End()
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
	// The decode side of the fpcalc pipe reads up to 12 minutes of media - an
	// I/O profile, demoted on both axes like every other background scan.
	ffmpegCmd, finishFFmpeg, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "chromaprint.decode"}, "ffmpeg", ffmpegArgs...)
	if err != nil {
		return nil, err
	}
	pipe, err := ffmpegCmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	ffmpegCmd.Stderr = nil
	if err := ffmpegCmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}
	// Suspend the audio decoder while real work runs; fpcalc then blocks on the
	// drained pipe, so the whole fingerprint pauses and resumes cleanly.
	stopPause := pauseProcessWhileBusy(ffmpegCmd.Process, c.busy)

	// fpcalc is not ffmpeg: no -threads injection (UnboundedThreads).
	fpCmd, finishFp, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: "chromaprint.fpcalc", UnboundedThreads: true},
		c.binary, "-raw", "-length", strconv.FormatFloat(lengthSec, 'f', 0, 64), "-")
	if err != nil {
		_ = ffmpegCmd.Process.Kill()
		_ = finishFFmpeg(ffmpegCmd.Wait(), nil) //nolint:errcheck // the refusal is the error being returned
		stopPause()
		return nil, err
	}
	fpCmd.Stdin = pipe
	out, fpErr := fpCmd.Output()
	fpErr = finishFp(fpErr, nil)
	// Wait for ffmpeg to finish so we don't leak the process; ignore its exit
	// status because EOF on stdout is normal once fpcalc closed the pipe.
	_ = finishFFmpeg(ffmpegCmd.Wait(), nil) //nolint:errcheck // EOF on stdout is normal once fpcalc closed the pipe
	stopPause()
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
