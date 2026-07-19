package mkvgo

import (
	"context"
	"fmt"

	"github.com/gravity-zero/mkvgo/mkv/ops"

	"github.com/evey-project/evey/internal/pkg/tracing"
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
// No error is returned for a non-EBML container - "unknown" is a legitimate,
// expected outcome for those, not a failure the caller should log or retry.
func (a *Adapter) AnalyzeFrameRateMode(ctx context.Context, path string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.AnalyzeFrameRateMode")
	defer span.End()

	if !IsEBMLContainer(path) {
		return "", nil
	}

	report, err := ops.Analyze(ctx, path)
	if err != nil {
		return "", fmt.Errorf("mkvgo analyze: %w", err)
	}
	for _, t := range report.Tracks {
		if t.Type == "video" {
			return t.FrameRateMode, nil
		}
	}
	return "", nil
}
