package mkvgo

import (
	"context"
	"slices"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
)

// FirstSubtitleCueStartsMs returns the start times (ms, ascending) of the
// blocks of an EBML file's first subtitle track, read from its Cues: the
// file's head only, where ffprobe reads every packet of the file. ok is false
// when the Cues cannot vouch for the whole track - no Cues for it, or a cue
// count that differs from the track's trusted NUMBER_OF_FRAMES statistic - and
// the caller then reads the packets. A file without a subtitle track is ok
// with no cues.
func FirstSubtitleCueStartsMs(ctx context.Context, path string) (cues []int64, ok bool) {
	if !IsEBMLContainer(path) {
		return nil, false
	}
	c, err := reader.OpenMeta(ctx, path, reader.WithCues(), reader.WithTags())
	if err != nil {
		return nil, false
	}
	var id uint64
	found := false
	for _, t := range c.Tracks {
		if t.Type == extmkv.SubtitleTrack {
			id, found = t.ID, true
			break
		}
	}
	if !found {
		return nil, true
	}
	for _, cue := range c.Cues {
		if cue.Track == id {
			cues = append(cues, cue.TimeMs)
		}
	}
	st, trusted := extmkv.TrustedTrackStatistics(c)[id]
	if len(cues) == 0 || !trusted || st.Frames != int64(len(cues)) {
		return nil, false
	}
	slices.Sort(cues)
	return cues, true
}
