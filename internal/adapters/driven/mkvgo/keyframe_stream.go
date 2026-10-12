package mkvgo

import (
	"context"
	"errors"
	"io"

	"github.com/gravity-zero/mkvgo/matroska"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// WriteKeyframeStream writes a keyframe-only Matroska of filePath to w for the
// trickplay sprites: mkvgo picks the first cued keyframe, then the first at
// least everyMs after the previous one kept, checks each is a picture a
// decoder can start on, and reads only those blocks. Matroska/WebM only; a
// Cues-less file errors (matroska.ErrNoKeyframeIndex), which the caller treats
// as the signal to scan the whole file instead.
func (a *Adapter) WriteKeyframeStream(ctx context.Context, filePath string, everyMs int64, w io.Writer) (matroska.KeyframeStats, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.WriteKeyframeStream")
	defer span.End()

	stats, err := matroska.WriteKeyframeStream(ctx, filePath, matroska.KeyframeSelect{EveryMs: everyMs}, w)
	if err != nil {
		span.RecordError(err)
	}
	return stats, err
}

// CuesMissKeyframes reports whether video keyframes sit uncued in the holes of
// filePath's index - a header-only walk of the holes alone. A hole the probe
// cannot conclude on counts as missing: the caller then scans the whole file,
// which is never wrong.
func (a *Adapter) CuesMissKeyframes(ctx context.Context, filePath string) (bool, error) {
	r, err := matroska.CueHealth(ctx, filePath)
	if errors.Is(err, matroska.ErrNotMatroska) {
		return false, nil // MP4: the sync-sample table lists every keyframe
	}
	if err != nil {
		return false, err
	}
	if err := matroska.ProbeCueHoles(ctx, filePath, r); err != nil {
		return false, err
	}
	for _, h := range r.Holes {
		if h.Content == "uncued-keyframes" || h.Content == "" {
			return true, nil
		}
	}
	return false, nil
}
