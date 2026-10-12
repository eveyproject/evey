package mkvgo

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/gravity-zero/mkvgo/mkv/ops"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// AnalyzeFrameRateMode classifies path's video track as constant or variable
// frame rate ("cfr" / "vfr"), or "" when unknown - a non-EBML container (MP4
// keeps its current probing paths), a container with fewer than 2 video
// frames, or one whose classification mkvgo could not derive. The walk is
// ops.Analyze's structural pass over block headers (proportional to block
// count, never to decoded volume - no sample is ever decoded).
//
// COST: seconds, not milliseconds - do not confuse this with the seek-index
// probe. "Head-only" is mkvgo's term for discarding block PAYLOADS; the walk
// itself still visits every block in the file, start to EOF, so wall time
// tracks the block count and therefore the runtime (~20s measured on a 2h 4K
// MKV). The millisecond-class probe is the Cues/stss read, which is a small
// localised index; this is a different, far heavier operation. Never call it
// on a request path.
//
// No error is returned for a non-EBML container, NOR for a structurally
// unreadable one - "unknown" is a legitimate, expected outcome for both, not a
// failure the caller should log or retry. Only a file we could not even open is
// reported as an error, because that alone might succeed on a later pass (a
// briefly unavailable mount, a permission being fixed). The distinction is what
// stops a permanently broken file from being re-walked on every sweep: the
// caller stamps an "unknown" and moves on, whereas an error is left to retry.
func (a *Adapter) AnalyzeFrameRateMode(ctx context.Context, path string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.AnalyzeFrameRateMode")
	defer span.End()

	if !IsEBMLContainer(path) {
		return "", nil
	}

	// Probed BEFORE the walk so an unreadable-vs-unavailable verdict rests on
	// the filesystem rather than on parsing mkvgo's error strings.
	if _, statErr := os.Stat(path); statErr != nil {
		return "", fmt.Errorf("mkvgo analyze: source unavailable: %w", statErr)
	}

	report, err := ops.Analyze(ctx, path)
	if err != nil {
		// The file is there and mkvgo cannot make sense of it (truncated,
		// corrupt VINT, damaged clusters). Re-walking it next sweep would fail
		// identically and cost another full-file pass, so report "unknown" and
		// let the caller record that the question has been settled.
		slog.Debug("mkvgo: frame-rate analysis gave up on an unreadable source",
			slog.String("path", path), slog.Any("err", err))
		return "", nil
	}
	for _, t := range report.Tracks {
		if t.Type == "video" {
			return t.FrameRateMode, nil
		}
	}
	return "", nil
}
