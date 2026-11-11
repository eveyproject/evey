package mkvgo

// Moving an audio track on the container timeline, losslessly.
//
// This is the same primitive that caused the mass retime damage, and that is
// exactly why it is the one that undoes it: the inverse of a known shift is
// exact. It works on block timecodes, so nothing is re-encoded and the codec
// does not matter - unlike laying silence in front of a raw stream, which only
// holds for constant-frame codecs and silences AAC or DTS outright.

import (
	"context"
	"fmt"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// TrackRetimer implements maintenance.TrackRetimer.
type TrackRetimer struct{}

// NewTrackRetimer builds the adapter. Stateless.
func NewTrackRetimer() *TrackRetimer { return &TrackRetimer{} }

// RetimeTracks shifts the given tracks by a signed number of seconds. No
// backup is kept: the caller's journal entry carries the shift, and its
// opposite is the way back - which is the whole point of not duplicating
// multi-gigabyte files for every repair.
func (t *TrackRetimer) RetimeTracks(ctx context.Context, path string, shiftSec map[uint64]float64) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.RetimeTracks")
	defer span.End()
	if len(shiftSec) == 0 {
		return nil
	}
	if !IsEBMLContainer(path) {
		return fmt.Errorf("retime: %q is not a Matroska container", path)
	}
	shift := make(map[uint64]int64, len(shiftSec))
	for track, sec := range shiftSec {
		shift[track] = int64(sec * 1e9)
	}
	if err := extmatroska.RetimeTracksReplace(ctx, path, shift); err != nil {
		return fmt.Errorf("retime: %w", err)
	}
	return nil
}

// AudioTrackNumbers lists the Matroska track number of each audio track, in
// the order the file declares them - the order ffmpeg counts a:0, a:1, ...,
// so index N here is the track the sync preview plays for audio index N.
func (t *TrackRetimer) AudioTrackNumbers(ctx context.Context, path string) ([]uint64, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.AudioTrackNumbers")
	defer span.End()
	if !IsEBMLContainer(path) {
		return nil, fmt.Errorf("audio tracks: %q is not a Matroska container", path)
	}
	c, err := reader.OpenMeta(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("audio tracks: %w", err)
	}
	var out []uint64
	for _, tr := range c.Tracks {
		if tr.Type == extmkv.AudioTrack {
			out = append(out, tr.ID)
		}
	}
	return out, nil
}
