package ffmpeg

// A transcode-seek rafale starts at `-ss N×3.000` but its first frame is the
// first SOURCE frame at or after that instant - on a 24000/1001 source that
// frame sits anywhere in [N×3.000, N×3.042). ffmpeg then restamps it to 0 and
// the rebase pump shifted it back by exactly N×3.000, so every seek rafale was
// early by that fraction of a frame: measured 2026-09-14 on a 4K remux,
// the cold rafale's segment 11 ended at 36.036 s and the seek rafale's segment
// 12 opened at 36.000 - a 36 ms overlap, two frames 6 ms apart at the join,
// and the whole rafale's video ahead of the absolutely-timed audio by the
// same 36 ms. The fix is to shift by where the frame really is.
//
// The landing is measured, not predicted: ffprobe lists the video packets from
// the keyframe before the target, and the first pts >= target is the frame
// ffmpeg's accurate seek keeps (its automatic trim filter drops every decoded
// frame with pts < -ss). No decode, a second of packet headers. Predicting it
// from the frame rate would have to replicate the container's timestamp
// rounding (Matroska stores milliseconds) and fail on VFR or offset sources.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/pkg/procfork"
)

// seekLandingProbeBinary is the bundled ffprobe, like every other probe here.
const seekLandingProbeBinary = "ffprobe"

// seekLandingWindowSec is how far past the target ffprobe reads. It only
// needs to reach the first frame at or after the target; one second covers
// any frame rate above 1 fps.
const seekLandingWindowSec = 1.0

// seekLandingSnapToleranceUs bounds the distance between a packet's stored
// pts and the ideal CFR grid time it is snapped to. Matroska keeps
// millisecond timestamps, so a 24000/1001 frame (k × 41708.33 µs) is stored
// up to 500 µs off; anything further is not rounding but a real irregularity
// (VFR, a dropped frame) and the stored pts is kept as is.
const seekLandingSnapToleranceUs = 600

// transcodeSeekLandingUs returns the absolute presentation time, in
// microseconds, of the first video frame a `-ss startSec` transcode session
// emits: the source frame with the smallest pts >= startSec. On a constant
// frame rate source the value is snapped to the exact frame grid (k × period),
// which is where the cold rafale's CFR output puts the same frame.
func transcodeSeekLandingUs(ctx context.Context, filePath string, startSec float64) (int64, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "hls.transcode_seek_landing"}, seekLandingProbeBinary,
		"-v", "error",
		"-select_streams", "v:0",
		"-read_intervals", fmt.Sprintf("%.3f%%+%.0f", startSec, seekLandingWindowSec),
		"-show_entries", "stream=r_frame_rate,avg_frame_rate:packet=pts_time",
		"-of", "json",
		filePath,
	)
	if err != nil {
		return 0, fmt.Errorf("seek landing probe: %w", err)
	}
	var parsed struct {
		Streams []struct {
			RFrameRate   string `json:"r_frame_rate"`
			AvgFrameRate string `json:"avg_frame_rate"`
		} `json:"streams"`
		Packets []struct {
			PTSTime string `json:"pts_time"`
		} `json:"packets"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return 0, fmt.Errorf("seek landing probe: parse: %w", err)
	}
	pts := make([]float64, 0, len(parsed.Packets))
	for _, pk := range parsed.Packets {
		v, err := strconv.ParseFloat(strings.TrimSpace(pk.PTSTime), 64)
		if err != nil {
			continue // a packet without pts (N/A) is not a candidate
		}
		pts = append(pts, v)
	}
	var fpsNum, fpsDen int64
	if len(parsed.Streams) > 0 {
		s := parsed.Streams[0]
		// Only a source that is CFR by its own account gets snapped: r_frame_rate
		// (the stream's base rate) and avg_frame_rate agree.
		if s.RFrameRate == s.AvgFrameRate {
			fpsNum, fpsDen = parseRational(s.RFrameRate)
		}
	}
	us, ok := seekLandingFromPackets(pts, startSec, fpsNum, fpsDen)
	if !ok {
		return 0, fmt.Errorf("seek landing probe: no video packet at or after %.3f s in a %.0f s window", startSec, seekLandingWindowSec)
	}
	return us, nil
}

// seekLandingFromPackets is the pure core of transcodeSeekLandingUs: among the
// packets' pts (seconds, any order - B-frames arrive out of presentation
// order), the smallest one >= startSec, in microseconds, snapped to the
// fpsNum/fpsDen frame grid when a grid is given and the pts sits within
// seekLandingSnapToleranceUs of it. ok=false when no packet qualifies.
func seekLandingFromPackets(pts []float64, startSec float64, fpsNum, fpsDen int64) (int64, bool) {
	// A packet stored a hair under the target by float formatting (ffprobe
	// prints 6 decimals) still counts: the comparison ffmpeg makes is on the
	// integer container timestamp, which is what the printed value rounds.
	const eps = 1e-6
	best, found := 0.0, false
	for _, p := range pts {
		if p < startSec-eps {
			continue
		}
		if !found || p < best {
			best, found = p, true
		}
	}
	if !found {
		return 0, false
	}
	us := int64(math.Round(best * 1e6))
	if fpsNum > 0 && fpsDen > 0 {
		// Frame index on the grid, then the grid time back in µs.
		k := math.Round(best * float64(fpsNum) / float64(fpsDen))
		gridUs := int64(math.Round(k * float64(fpsDen) * 1e6 / float64(fpsNum)))
		if d := gridUs - us; d >= -seekLandingSnapToleranceUs && d <= seekLandingSnapToleranceUs {
			us = gridUs
		}
	}
	return us, true
}

// parseRational reads ffprobe's "24000/1001" form into its integer parts;
// (0, 0) when unusable (empty, "0/0", malformed).
func parseRational(raw string) (num, den int64) {
	n, d, ok := strings.Cut(strings.TrimSpace(raw), "/")
	if !ok {
		return 0, 0
	}
	num, err1 := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
	den, err2 := strconv.ParseInt(strings.TrimSpace(d), 10, 64)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, 0
	}
	return num, den
}

// rebaseDeltaUs is the shift the rebase pump applies to a session's 0-based
// fMP4 timeline. Copy sessions start on a keyframe the plan already names
// exactly (StartSec is that keyframe's time) and audio seeks are trimmed to
// the sample, so both shift by StartSec itself. A transcode seek shifts by the
// measured frame landing; when the probe fails the grid start is used, the
// pre-fix behaviour, and the session is logged so the ≤ 1-frame join error
// is visible rather than silent.
func rebaseDeltaUs(ctx context.Context, filePath, videoDecision string, startSec float64) int64 {
	grid := int64(math.Round(startSec * 1e6))
	if videoDecision == "copy" || startSec <= 0 {
		return grid
	}
	us, err := transcodeSeekLandingUs(ctx, filePath, startSec)
	if err != nil {
		slog.WarnContext(ctx, "hls: transcode-seek frame landing unknown, rebasing on the segment grid (join may be off by up to one frame)",
			slog.String("media_path", filePath), slog.Float64("start_sec", startSec), slog.Any("err", err))
		return grid
	}
	return us
}
