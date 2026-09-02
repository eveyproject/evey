package maintenance

// Undoing a repair from its journal entry, with no copy of the original.
//
// Keeping a duplicate of every repaired file costs a library its disk: 98 files
// on one July night would have been 400 GB of copies, and the ones that WERE
// kept got purged by the rolling window anyway. The bargain here is different -
// write the transformation down precisely enough to replay it backwards.
//
// It holds for the operations whose inverse is exact (a track shifted by a
// known amount, a file renamed) and it does not hold for the one that throws
// bytes away. That one still demands a real backup, and says so.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

var (
	// ErrNoRepairToUndo means the journal has nothing recorded for this file.
	// A clean answer, not a failure: a file nobody repaired is the normal case.
	ErrNoRepairToUndo = errors.New("no repair recorded for this file")
	// ErrNotReversible means the last repair cannot be replayed backwards -
	// either it discarded bytes, or the primitive that would replay it is not
	// wired. Never silently treated as success.
	ErrNotReversible = errors.New("the last repair on this file cannot be undone in software")
	// ErrJournalSchema means the entry was written by a build whose payload
	// shape this one does not know. Refusing beats guessing: a rollback driven
	// by a misread record corrupts the file it was meant to save.
	ErrJournalSchema = errors.New("this repair was recorded by a newer version and cannot be read safely")
)

// RepairHistoryOf returns what has been done to one file, newest first. The
// read side of the journal, and what the UI shows next to a media entry.
func (s *SeekIndexService) RepairHistoryOf(ctx context.Context, path string, limit int) ([]RepairRecord, error) {
	if s.journal == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	return s.journal.RepairHistory(ctx, path, limit)
}

// RepairHistoryOfMedia is RepairHistoryOf addressed by media id, which is what
// the admin UI holds.
func (s *SeekIndexService) RepairHistoryOfMedia(ctx context.Context, mediaID string, limit int) ([]RepairRecord, error) {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("repair history: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return nil, fmt.Errorf("repair history: media %s has no file", mediaID)
	}
	return s.RepairHistoryOf(ctx, f.Path, limit)
}

// UndoLastRepairOfMedia is UndoLastRepair addressed by media id. Returns the
// shift it applied so the caller can say what it did rather than "done".
func (s *SeekIndexService) UndoLastRepairOfMedia(ctx context.Context, mediaID string) (map[uint64]float64, error) {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("undo repair: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return nil, fmt.Errorf("undo repair: media %s has no file", mediaID)
	}
	history, herr := s.RepairHistoryOf(ctx, f.Path, 20)
	if herr != nil {
		return nil, herr
	}
	last, ok := lastUndoable(history)
	if !ok {
		// Fall through to UndoLastRepair, which distinguishes "nothing
		// recorded" from "recorded but not replayable" and says which.
		return nil, s.UndoLastRepair(ctx, f.Path)
	}
	if err := s.UndoLastRepair(ctx, f.Path); err != nil {
		return nil, err
	}
	return last.InverseShift(), nil
}

// UndoLastRepair replays the file's most recent reversible repair backwards.
// The undo is itself journalled, so the history reads as a chain rather than a
// file that changed twice for no stated reason.
func (s *SeekIndexService) UndoLastRepair(ctx context.Context, path string) error {
	if s.journal == nil {
		return ErrNoRepairToUndo
	}
	history, err := s.journal.RepairHistory(ctx, path, 20)
	if err != nil {
		return fmt.Errorf("undo repair: read journal: %w", err)
	}

	last, ok := lastUndoable(history)
	if !ok {
		if len(history) == 0 {
			return ErrNoRepairToUndo
		}
		// An entry we cannot read is a different refusal from one we can read
		// and cannot replay - the operator deserves to know which.
		for _, h := range history {
			if h.Schema > RepairJournalSchema {
				return ErrJournalSchema
			}
		}
		return ErrNotReversible
	}

	rec := RepairRecord{
		MediaID: last.MediaID, Path: path, Op: last.Op, Started: time.Now(),
		SizeBefore: fileSizeOf(path),
		Evidence: RepairEvidence{
			Method: "operator-undo",
			Detail: fmt.Sprintf("undoes the repair of %s", last.Started.UTC().Format(time.RFC3339)),
		},
	}

	switch last.Op {
	case RepairAudioRetime:
		if s.retimer == nil {
			return ErrNotReversible
		}
		inverse := last.InverseShift()
		rec.ShiftSec = inverse
		// A retime moves audio bytes on the timeline; it is judged on the
		// whole track, and a damaged result is put back by replaying the
		// shift that was being undone - the inverse of an exact shift is
		// exact in both directions.
		if err := s.RetimeTracksGuarded(ctx, path, inverse); err != nil {
			rec.Outcome, rec.Error = "failed", err.Error()
			if errors.Is(err, ErrAudioIntegrityLost) {
				rec.Outcome = "rolled_back"
			}
			s.journalRepair(ctx, rec)
			return fmt.Errorf("undo repair: %w", err)
		}
	default:
		return ErrNotReversible
	}

	rec.Outcome = "applied"
	rec.SizeAfter = fileSizeOf(path)
	s.journalRepair(ctx, rec)
	s.logger.Info("seek-index: repair undone from the journal",
		slog.String("path", path), slog.String("op", string(last.Op)))
	return nil
}

// lastUndoable picks the newest entry that can actually be replayed backwards,
// skipping the ones that only describe (a skipped file, a failure) and the ones
// written by a shape this build does not know.
func lastUndoable(history []RepairRecord) (RepairRecord, bool) {
	for _, h := range history {
		if h.Schema > RepairJournalSchema {
			continue
		}
		if h.Op == RepairAudioRetime && h.Reversible() {
			return h, true
		}
	}
	return RepairRecord{}, false
}
