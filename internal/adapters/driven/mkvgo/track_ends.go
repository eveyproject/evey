package mkvgo

import (
	"context"
	"fmt"

	"github.com/gravity-zero/mkvgo/matroska"
)

// TrackEndMs measures how long the content of an EBML file really lasts: the
// latest end among its tracks (statistics tags when they describe this file,
// else a bounded, header-only walk of the tail - matroska.TrackEnds), minus
// where its timeline starts. It is the duration of a file whose Info element
// declares none - a Matroska written as a live stream (ffmpeg `-live 1`, a
// capture never finalised) carries no Duration, and mkvgo faithfully reports
// 0 for it. A capture cut mid-stream starts late (CELLAR test4: first Cluster
// at 12.345 s): its length is end - start, 68.4 s, what ffmpeg plays, not the
// 80.8 s end. 0 when nothing is known.
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
	if end <= rep.StartMs {
		return 0, nil
	}
	return end - rep.StartMs, nil
}
