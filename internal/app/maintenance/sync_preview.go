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

	"github.com/eveyproject/evey/internal/domain/transcoding"
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

// ErrShiftOutOfRange is an operator shift past SyncPreviewMaxShiftSec: a
// mistyped number, refused before it touches a file.
var ErrShiftOutOfRange = fmt.Errorf("a shift beyond %.0fs is not something this tool will move a track by", SyncPreviewMaxShiftSec)

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
	// Picture is the player's picture decision for this file (accelerator,
	// tone-map, deinterlace, SAR): the clip is drawn the way the viewer sees
	// the film, by the same route. The zero value draws it in software with
	// no correction - what a caller without the resolver can offer.
	Picture transcoding.PicturePlan
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

// ErrAudioTrackMissing is an audio index past the file's last audio track: the
// caller named a track the file does not have, refused before anything is
// written.
var ErrAudioTrackMissing = errors.New("the file has no such audio track")

// ErrAudioTracksUnavailable means no lister is wired, so an audio index cannot
// be turned into the track number a retime needs.
var ErrAudioTracksUnavailable = errors.New("audio track lookup is not available on this server")

// AudioTrackLister reads a file's audio tracks as Matroska track numbers, in
// the order the preview counts them (index 0 = the first audio track).
// Satisfied by the mkvgo adapter; optional.
type AudioTrackLister interface {
	AudioTrackNumbers(ctx context.Context, path string) ([]uint64, error)
}

// WithAudioTrackLister wires the audio track lookup. Optional and chainable.
func (s *SeekIndexService) WithAudioTrackLister(l AudioTrackLister) *SeekIndexService {
	s.audioTracks = l
	return s
}

// ApplyOperatorShiftToAudio is ApplyOperatorShift for the track the preview
// played: audioIndex counts audio tracks only, the way the clip picks one.
// The Matroska number is read from the file because it cannot be derived:
// tracks are numbered across every type, so the second audio track of a file
// with subtitles in between is not track 2.
func (s *SeekIndexService) ApplyOperatorShiftToAudio(ctx context.Context, mediaID string, audioIndex int, shiftSec float64) error {
	if s.audioTracks == nil {
		return ErrAudioTracksUnavailable
	}
	if shiftSec > SyncPreviewMaxShiftSec || shiftSec < -SyncPreviewMaxShiftSec {
		return fmt.Errorf("apply shift: %+.3fs: %w", shiftSec, ErrShiftOutOfRange)
	}
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("apply shift: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("apply shift: media %s has no file", mediaID)
	}
	numbers, err := s.audioTracks.AudioTrackNumbers(ctx, f.Path)
	if err != nil {
		return fmt.Errorf("apply shift: list audio tracks of %s: %w", mediaID, err)
	}
	if audioIndex < 0 || audioIndex >= len(numbers) {
		return fmt.Errorf("apply shift: audio index %d of %d: %w", audioIndex, len(numbers), ErrAudioTrackMissing)
	}
	return s.ApplyOperatorShift(ctx, mediaID, numbers[audioIndex], shiftSec)
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
		return fmt.Errorf("apply shift: %+.3fs: %w", shiftSec, ErrShiftOutOfRange)
	}
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("apply shift: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("apply shift: media %s has no file", mediaID)
	}

	// A rewrite of the library disk like any batch repair: it holds the job
	// slot and stands aside for a viewer, and is refused before anything is
	// journalled when it cannot have either.
	return s.runInSlot(JobAudioRetime, f.Path, func(ctx context.Context) error {
		rec := RepairRecord{
			MediaID: mediaID, Path: f.Path, Op: RepairAudioRetime, Started: s.now(),
			SizeBefore: fileSizeOf(f.Path),
			ShiftSec:   map[uint64]float64{track: shiftSec},
			Evidence: RepairEvidence{
				Method: "operator",
				Detail: "judged on a preview clip and applied by hand",
			},
		}
		// One call, one implementation of the rule: the retime, the whole-track
		// integrity check around it, and the inverse shift that puts the file back
		// if sound went missing. No backup exists by design, so that inverse IS
		// the way back.
		if err := s.RetimeTracksGuarded(ctx, f.Path, rec.ShiftSec); err != nil {
			// "rolled_back" only when the file on disk really is the original
			// again - ErrOriginalRestored says so. This used to test for
			// ErrAudioLost, which a retime cannot return (that is the SAMPLED
			// guard's sentinel; a retime is judged by the exhaustive one), so
			// every restored retime was filed as a plain failure.
			rec.Outcome = repairOutcomeAfterGuard(err)
			rec.Error = err.Error()
			s.journalRepair(ctx, rec)
			return fmt.Errorf("apply shift: %w", err)
		}
		rec.Outcome = "applied"
		rec.SizeAfter = fileSizeOf(f.Path)
		s.journalRepair(ctx, rec)
		s.refreshFileState(ctx, f.Path)
		return nil
	})
}
