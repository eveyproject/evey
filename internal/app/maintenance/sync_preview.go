package maintenance

// Seeing the defect before repairing it.
//
// Nothing this server can measure answers the only question that matters about
// a desynchronised film: does the sound match the PICTURE? Comparing a file's
// two audio tracks says whether they agree with each other, never which one
// follows the image - a whole evening of measurements got that wrong on two
// real films, and the answer came in ten seconds from someone watching a clip.
//
// So the tool is a clip. A few seconds of picture with one chosen audio track,
// optionally shifted by a candidate amount, rendered small enough to play in a
// browser. The operator watches, adjusts, watches again, and hands over the
// number their ear found - which is then applied and journalled as exactly
// that: an operator's judgement, not a measurement.
//
// It is rendered server-side because a browser cannot play what these files
// are: 4K HEVC in Matroska with AC-3 or DTS audio. A <video> pointed at the
// original would show nothing at all.

import (
	"context"
	"errors"
	"fmt"
)

// SyncPreviewMaxSec bounds a preview. Long enough to judge lip sync on a line
// of dialogue, short enough that rendering it costs a couple of seconds and
// nobody is tempted to watch the film through it.
const SyncPreviewMaxSec = 45

// SyncPreviewMaxShiftSec bounds the candidate offset the operator can try. Real
// dub misalignments observed here run to a dozen seconds; past a minute the
// question is not a shift any more.
const SyncPreviewMaxShiftSec = 60.0

// ErrPreviewUnavailable means no renderer is wired, so the server cannot build
// a clip. Said plainly rather than returned as an empty video.
var ErrPreviewUnavailable = errors.New("sync preview rendering is not available on this server")

// SyncPreviewRequest is one clip to build.
type SyncPreviewRequest struct {
	Path string
	// AtSec is where in the film the clip starts.
	AtSec float64
	// LenSec is its length, capped at SyncPreviewMaxSec.
	LenSec float64
	// Track is the audio track to hear, as an ffmpeg audio index (0 = first).
	Track int
	// ShiftSec delays that track by this much against the picture (negative
	// advances it). Zero shows the file as it is - the "before" a judgement
	// needs to be made against.
	ShiftSec float64
}

// SyncPreviewRenderer builds a small, browser-playable clip of one moment of a
// file, with one audio track, optionally shifted. Satisfied by the ffmpeg
// adapter; optional.
type SyncPreviewRenderer interface {
	RenderSyncPreview(ctx context.Context, req SyncPreviewRequest) (mp4 []byte, err error)
}

// WithSyncPreviewRenderer wires the preview builder. Optional and chainable.
func (s *SeekIndexService) WithSyncPreviewRenderer(r SyncPreviewRenderer) *SeekIndexService {
	s.preview = r
	return s
}

// normalizeSyncPreview clamps a request into what the renderer will accept. The
// caller is a UI control an operator drags, so the bounds live here rather than
// trusting whatever arrives on the wire.
func normalizeSyncPreview(req SyncPreviewRequest) SyncPreviewRequest {
	if req.LenSec <= 0 || req.LenSec > SyncPreviewMaxSec {
		req.LenSec = SyncPreviewMaxSec
	}
	if req.AtSec < 0 {
		req.AtSec = 0
	}
	if req.Track < 0 {
		req.Track = 0
	}
	if req.ShiftSec > SyncPreviewMaxShiftSec {
		req.ShiftSec = SyncPreviewMaxShiftSec
	}
	if req.ShiftSec < -SyncPreviewMaxShiftSec {
		req.ShiftSec = -SyncPreviewMaxShiftSec
	}
	// A clip that would start before the film does, because the shift pulls the
	// audio window backwards, is moved forward instead of failing: the operator
	// asked to see a moment, not to be told about a boundary.
	if req.ShiftSec > 0 && req.AtSec < req.ShiftSec {
		req.AtSec = req.ShiftSec
	}
	return req
}

// SyncPreviewOfMedia renders a clip for one media item.
func (s *SeekIndexService) SyncPreviewOfMedia(ctx context.Context, mediaID string, req SyncPreviewRequest) ([]byte, error) {
	if s.preview == nil {
		return nil, ErrPreviewUnavailable
	}
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("sync preview: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return nil, fmt.Errorf("sync preview: media %s has no file", mediaID)
	}
	req.Path = f.Path
	return s.preview.RenderSyncPreview(ctx, normalizeSyncPreview(req))
}

// ApplyOperatorShift moves one audio track by the amount a person judged by
// ear, and journals it as exactly that. This is the one path where a media
// rewrite is justified by a human rather than a measurement, and the entry
// says so - "operator" is evidence too, and better evidence than the declared
// offset that caused the 2026-07-13 incident.
func (s *SeekIndexService) ApplyOperatorShift(ctx context.Context, mediaID string, track uint64, shiftSec float64) error {
	if s.retimer == nil {
		return ErrNotReversible
	}
	if shiftSec > SyncPreviewMaxShiftSec || shiftSec < -SyncPreviewMaxShiftSec {
		return fmt.Errorf("apply shift: %+.3fs is beyond the %.0fs this tool will move a track",
			shiftSec, SyncPreviewMaxShiftSec)
	}
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("apply shift: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("apply shift: media %s has no file", mediaID)
	}

	rec := RepairRecord{
		MediaID: mediaID, Path: f.Path, Op: RepairAudioRetime, Started: s.now(),
		SizeBefore: fileSizeOf(f.Path),
		ShiftSec:   map[uint64]float64{track: shiftSec},
		Evidence: RepairEvidence{
			Method: "operator",
			Detail: "judged on a preview clip and applied by hand",
		},
	}
	// The same guard every other rewrite gets, and the same implementation:
	// a repair must never cost the file its sound, whoever asked for it. No
	// backup exists by design, so the way back is the inverse shift.
	before, haveBaseline := s.audioIntegrityBaseline(ctx, f.Path)
	rec.AudibleBefore = len(before)

	if err := s.retimer.RetimeTracks(ctx, f.Path, rec.ShiftSec); err != nil {
		rec.Outcome, rec.Error = "failed", err.Error()
		s.journalRepair(ctx, rec)
		return fmt.Errorf("apply shift: %w", err)
	}
	if haveBaseline {
		if err := s.guardAudioIntegrity(ctx, f.Path, before, func() error {
			return s.retimer.RetimeTracks(ctx, f.Path, map[uint64]float64{track: -shiftSec})
		}); err != nil {
			rec.Outcome, rec.Error = "rolled_back", err.Error()
			s.journalRepair(ctx, rec)
			return fmt.Errorf("apply shift: %w", err)
		}
	}
	rec.Outcome = "applied"
	rec.SizeAfter = fileSizeOf(f.Path)
	s.journalRepair(ctx, rec)
	s.refreshFileState(ctx, f.Path)
	return nil
}
