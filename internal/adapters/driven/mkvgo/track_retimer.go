package mkvgo

// Moving an audio track on the container timeline, losslessly.
//
// This is the same primitive that caused the 2026-07-13 damage, and that is
// exactly why it is the one that undoes it: the inverse of a known shift is
// exact. It works on block timecodes, so nothing is re-encoded and the codec
// does not matter - unlike laying silence in front of a raw stream, which only
// holds for constant-frame codecs and silences AAC or DTS outright.

import (
	"context"
	"fmt"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"

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
