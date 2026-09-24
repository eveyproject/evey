package mkvgo

import (
	"context"
	"fmt"

	"github.com/gravity-zero/mkvgo/matroska"
)

// TrackEndMs measures where the content of an EBML file really ends: the
// latest end among its tracks, from the statistics tags when they describe
// this file, else a bounded, header-only walk of the tail (matroska.TrackEnds).
// It is the duration of a file whose Info element declares none - a Matroska
// written as a live stream (ffmpeg `-live 1`, a capture that was never
// finalised) carries no Duration, and mkvgo faithfully reports 0 for it. 0
// when nothing is known.
func TrackEndMs(ctx context.Context, path string) (int64, error) {
	rep, err := matroska.TrackEnds(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("mkvgo track ends %q: %w", path, err)
	}
	if rep == nil {
		return 0, nil
	}
	end := rep.VideoEndMs
	for _, e := range rep.Ends {
		// "walk-bound" is an upper bound on a silent track, not an end seen;
		// the picture's end and any block actually seen are.
		if e.Source == "" || e.Source == "walk-bound" {
			continue
		}
		end = max(end, e.EndMs)
	}
	return end, nil
}
