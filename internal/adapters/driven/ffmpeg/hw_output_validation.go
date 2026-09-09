package ffmpeg

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"
)

// Hardware encoders can fail SILENTLY: exit 0, healthy FPS, and yet visually
// broken frames (solid green/purple output, mishandled 10-bit, wrong SAR) on
// the buggy driver/GPU combos that define the long tail of Linux hardware
// transcoding. Both calibration gates look away from the pixels - the stderr
// classifier sees FailureNone and the bench only counts frames per second -
// so a corrupt path would be certified and production would serve garbage
// with confidence. Graceful degradation cannot demote what it cannot detect.
//
// This gate closes that hole at calibration time: encode a short probe
// through the exact candidate path, decode a few frames back, and compare
// each with the software-decoded source frame at the same timestamp using the
// same perceptual hash as intro detection (pHashDCT). A faithful lossy encode
// of the same picture lands within a few bits; a corrupted path lands tens of
// bits away.

const (
	// hwValidateEncodeSeconds bounds the probe encode: long enough to cover
	// every sampling offset below, short enough to stay negligible next to
	// the bench itself.
	hwValidateEncodeSeconds = 4
	// hwValidateHeight matches the lowest hardware rung - the cheapest shape
	// that still runs the candidate scaler+encoder pipeline for real.
	hwValidateHeight = 720
	// hwCorruptHammingThreshold is the pHash Hamming distance (0..64) at or
	// beyond which a decoded output frame no longer depicts the source frame.
	// Faithful re-encodes of the same picture measure 0-6 bits even at low
	// bitrate; corrupted output (inverted planes, chroma garbage, solid
	// frames) measures well past 20; unrelated pictures average ~32. 16 sits
	// between the two populations with margin on both sides.
	//
	// ⚠️ Chosen on the dev rig, where only the MECHANISM can be proven. The
	// margin must be validated on real target hardware (weak-NAS QSV/VAAPI)
	// before this number is trusted blindly - see the backlog entry.
	hwCorruptHammingThreshold = 16
)

// hwValidateOffsets are the timestamps (seconds) sampled from the probe encode.
// Two frames far apart catch both a corrupt-from-frame-one path and a path that
// degrades after the first GOP.
//
// They are DERIVED from the probe output's own duration rather than fixed,
// because the encode is bounded by hwValidateEncodeSeconds but the sample can be
// shorter: prod, 2026-09-09, ran a fixed {1.0, 3.0} against a 2.88 s
// sample.hevc, so the 3 s extraction failed every single time. An extraction
// failure means "the gate could not conclude", so every hardware path was kept
// on its speed measurement alone - the exact thing this gate exists to prevent.
//
// Below ~1 s there is no room for two distinct pictures and one is returned;
// a single frame still catches corrupt-from-frame-one, which is the common
// failure. dur <= 0 (no verdict from the probe) returns nothing, and the caller
// treats that as "cannot conclude" rather than a pass.
func hwValidateOffsets(dur float64) []float64 {
	if dur <= 0 {
		return nil
	}
	// Inset from both ends: 0 is a keyframe on every path (never diagnostic of
	// a mid-GOP problem) and the last frame is where a truncated write lands.
	first := dur * 0.25
	last := dur * 0.75
	if last-first < 0.4 {
		return []float64{first}
	}
	return []float64{first, last}
}

// hwVisualVerdict is the outcome of one decode-back validation.
type hwVisualVerdict struct {
	// OK is false when any sampled frame diverges at or beyond
	// hwCorruptHammingThreshold - the path serves garbage.
	OK bool
	// WorstHamming is the largest observed distance, for the log line and the
	// admin calibration evidence.
	WorstHamming int
}

// validateHWVisualOutput encodes a short probe through the candidate hardware
// path (same input args / upload chain / scaler / encoder shape as the bench
// rung that passed) and decode-back-compares sampled frames against the
// software-decoded source.
//
// An error means the gate itself could not conclude (encode or extraction
// failed); the caller keeps the path and logs - only a MEASURED divergence
// demotes, so a fluke of the gate can never brick working hardware.
func (r *BenchmarkRunner) validateHWVisualOutput(ctx context.Context, sample string, fallback bool, encoder string, extraInputArgs []string, uploadChain string) (hwVisualVerdict, error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(hwValidateEncodeSeconds*3)*time.Second+10*time.Second)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "evey-hw-validate-*")
	if err != nil {
		return hwVisualVerdict{}, fmt.Errorf("hw validate: temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	outPath := filepath.Join(tmpDir, "probe.mp4")

	// Encode: mirrors the runMultiVariant single-rung shape (same scaler and
	// preset per encoder family) but writes a real file - the bench encodes to
	// `-f null -`, which leaves nothing to inspect.
	args := []string{"-hide_banner", "-nostats"}
	args = append(args, extraInputArgs...)
	args = append(args, r.validateInputArgs(sample, fallback)...)
	hwScale, hwPreset := "", ""
	switch {
	case strings.HasSuffix(encoder, "_nvenc"):
		hwScale, hwPreset = "scale_cuda", "p4"
	case strings.HasSuffix(encoder, "_vaapi"):
		hwScale = "scale_vaapi"
	case strings.HasSuffix(encoder, "_qsv"):
		hwScale, hwPreset = "scale_qsv", "veryfast"
	}
	head := "[0:v]"
	if uploadChain != "" {
		head = "[0:v]" + uploadChain + ","
	}
	scale := hwScale
	if scale == "" {
		scale = "scale"
	}
	fc := fmt.Sprintf("%s%s=-2:%d[o0]", head, scale, hwValidateHeight)
	args = append(args, "-filter_complex", fc, "-map", "[o0]", "-c:v", encoder)
	switch {
	case hwScale != "":
		if hwPreset != "" {
			args = append(args, "-preset", hwPreset)
		}
	default:
		args = append(args, "-preset", "veryfast", "-pix_fmt", "yuv420p")
	}
	args = append(args, "-an", "-sn", "-f", "mp4", "-y", outPath)
	if out, err := procfork.CombinedOutput(runCtx, procfork.Options{Class: procfork.Production, Site: "hw_validate.encode", UnboundedThreads: true}, r.ffmpegPath, args...); err != nil {
		return hwVisualVerdict{}, fmt.Errorf("hw validate: probe encode failed: %w (%s)", err, tailString(string(out), 200))
	}

	// The probe output is the window BOTH extractions have to land inside: the
	// source is bounded by the same -t, so anything the output holds the source
	// holds too.
	probeDur, durErr := mediaDurationSec(runCtx, r.ffprobeBin(), outPath)
	if durErr != nil {
		return hwVisualVerdict{}, fmt.Errorf("hw validate: probe output duration: %w", durErr)
	}
	offsets := hwValidateOffsets(probeDur)
	if len(offsets) == 0 {
		return hwVisualVerdict{}, fmt.Errorf("hw validate: probe output has no usable duration (%.3fs)", probeDur)
	}

	worst := 0
	for i, off := range offsets {
		// Reference: SOFTWARE decode of the source, deliberately without the
		// candidate path's input args - a decode-side corruption applied to
		// both sides would cancel itself out.
		srcHash, err := r.extractFrameHash(runCtx, r.validateInputArgs(sample, fallback), off, filepath.Join(tmpDir, fmt.Sprintf("src_%d.png", i)))
		if err != nil {
			return hwVisualVerdict{}, fmt.Errorf("hw validate: source frame at %.1fs: %w", off, err)
		}
		outHash, err := r.extractFrameHash(runCtx, []string{"-i", outPath}, off, filepath.Join(tmpDir, fmt.Sprintf("out_%d.png", i)))
		if err != nil {
			return hwVisualVerdict{}, fmt.Errorf("hw validate: output frame at %.1fs: %w", off, err)
		}
		if d := hammingDistance(srcHash, outHash); d > worst {
			worst = d
		}
	}
	return hwVisualVerdict{OK: worst < hwCorruptHammingThreshold, WorstHamming: worst}, nil
}

// validateInputArgs returns the ffmpeg input flags for the probe source,
// bounded to the validation window. Mirrors runMultiVariant's fallback
// handling: with no usable sample the bench runs on lavfi testsrc, whose
// frames are deterministic and therefore reproducible for the reference
// extraction too.
func (r *BenchmarkRunner) validateInputArgs(sample string, fallback bool) []string {
	if fallback {
		return []string{"-f", "lavfi", "-i",
			fmt.Sprintf("testsrc=duration=%d:size=3840x2160:rate=24", hwValidateEncodeSeconds)}
	}
	return []string{"-t", fmt.Sprintf("%d", hwValidateEncodeSeconds), "-i", sample}
}

// extractFrameHash decodes exactly one frame at offset seconds (output-side
// -ss ⇒ frame-accurate: ffmpeg decodes and discards up to the target, so the
// source and the re-encode land on the same picture) and returns its pHash.
func (r *BenchmarkRunner) extractFrameHash(ctx context.Context, inputArgs []string, offset float64, dst string) (uint64, error) {
	args := []string{"-hide_banner", "-nostats"}
	args = append(args, inputArgs...)
	args = append(args, "-ss", fmt.Sprintf("%.3f", offset), "-frames:v", "1", "-f", "image2", "-y", dst)
	if out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Production, Site: "hw_validate.frame_hash", UnboundedThreads: true}, r.ffmpegPath, args...); err != nil {
		return 0, fmt.Errorf("frame extract: %w (%s)", err, tailString(string(out), 200))
	}
	f, err := os.Open(dst)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("decode %s: %w", filepath.Base(dst), err)
	}
	b := img.Bounds()
	return pHashDCT(img, b.Min.X, b.Min.Y, b.Max.X, b.Max.Y), nil
}
