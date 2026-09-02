package maintenance

// What a repair did to a file, written down before anyone needs it.
//
// On 2026-07-13 a repair batch rewrote 98 media files between 22:24 and 01:11,
// pulling every audio track that declared a start offset back onto the picture.
// Several of those offsets were deliberate dub alignments, and the films came
// out desynchronised. The damage surfaced SEVEN WEEKS later, and by then the
// only surviving evidence was the files' mtime: the audit log held the
// operator's page view of /admin/seek-index and nothing about the job itself.
// The amount removed from each track was gone - not in the file (a shift
// preserves a track's length), not in the database, not in any log - so most of
// those files can be named as victims but never restored.
//
// This journal is the answer to that. Every rewrite writes one entry saying
// what the file looked like, what was done to it, on what evidence, and what it
// looked like afterwards. Three things follow from it, and only from it:
//
//   - a repair can be UNDONE from the entry alone, even with no backup left:
//     the shift is written down, so applying its opposite restores the file;
//   - a genuine repair can be told apart from damage: the entry carries the
//     PROOF that justified it (what was measured, how confidently), and an
//     action taken without proof is visible as such;
//   - a whole batch can be reviewed after the fact by job, instead of guessing
//     from file timestamps.

import (
	"context"
	"time"
)

// RepairOp names what was done to the bytes. One value per distinct
// transformation - a reader must never have to infer it from the details.
type RepairOp string

const (
	// RepairReindex rebuilt the seek index (Cues) of a file.
	RepairReindex RepairOp = "reindex"
	// RepairResync rewrote a file with corrupt stretches skipped.
	RepairResync RepairOp = "resync_damaged"
	// RepairContainerFix renamed a mislabelled .mkv to .mp4.
	RepairContainerFix RepairOp = "container_fix"
	// RepairAudioRetime moved an audio track on the container timeline. This
	// is the operation that caused the 2026-07-13 incident; it is no longer
	// performed automatically, and any future use MUST be journalled.
	RepairAudioRetime RepairOp = "audio_retime"
	// RepairRollback put a backup back in place.
	RepairRollback RepairOp = "rollback"
)

// TrackDelay is one audio track's declared start offset at a point in time.
// Recording the whole set is what makes an audio retime reversible: the value
// a retime removes exists nowhere else once the file is rewritten.
type TrackDelay struct {
	Track    uint64  `json:"track"`
	Language string  `json:"language,omitempty"`
	DelaySec float64 `json:"delay_sec"`
}

// RepairEvidence is what justified acting. An entry with no evidence is an
// action taken on a declaration rather than on a measurement - exactly what
// happened in July - and it must be visible as such to anyone reading back.
type RepairEvidence struct {
	// Method names how the defect was established ("declared-offset",
	// "track-coherence", "operator"). "declared-offset" alone is the weak case.
	Method string `json:"method"`
	// LagSec and Confidence carry the coherence measurement when one ran.
	// Confidence below AudioCoherenceMinConfidence means the measurement was
	// discarded, and the entry says so rather than hiding it.
	LagSec     float64 `json:"lag_sec,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	// Detail is free text for what does not fit above (the diagnoser's own
	// reason, an operator's stated intent).
	Detail string `json:"detail,omitempty"`
}

// RepairJournalSchema is the version of the payload shape below. It is stored
// with every entry so a future reader knows how to interpret one written by an
// older build: the whole point of this journal is to be read years later, by
// code that has moved on, to undo something. A reader that meets a version it
// does not know must refuse to act on the entry rather than guess.
const RepairJournalSchema = 1

// RepairRecord is one file's rewrite, start to finish.
type RepairRecord struct {
	// Schema is RepairJournalSchema at write time. Never omitted.
	Schema int `json:"schema"`
	MediaID string   `json:"media_id,omitempty"`
	Path    string   `json:"path"`
	Op      RepairOp `json:"op"`
	// Job identifies the batch this belonged to, so a whole night's work can
	// be reviewed as one thing instead of reconstructed from file timestamps.
	Job     string    `json:"job,omitempty"`
	Started time.Time `json:"started"`

	// SizeBefore and SizeAfter bracket the rewrite. A file whose size did not
	// change was skipped, whatever else the entry says.
	SizeBefore int64 `json:"size_before"`
	SizeAfter  int64 `json:"size_after"`

	// DelaysBefore is every audio track's declared offset BEFORE the rewrite.
	// This is the field that would have made the July batch reversible.
	DelaysBefore []TrackDelay `json:"delays_before,omitempty"`
	// ShiftSec is what was applied, per track, signed (positive = later).
	// Its opposite restores the file even when no backup survives.
	ShiftSec map[uint64]float64 `json:"shift_sec,omitempty"`
	// DelaysAfter is the same measurement once the rewrite landed, so the
	// entry proves what it claims instead of asserting it.
	DelaysAfter []TrackDelay `json:"delays_after,omitempty"`

	// AudibleBefore/After are the guard's counts. Equal counts mean no
	// soundtrack was lost; a drop is what triggers the automatic rollback.
	AudibleBefore int `json:"audible_before"`
	AudibleAfter  int `json:"audible_after"`

	Evidence RepairEvidence `json:"evidence"`
	// BackupPath is where the original went, "" when none was kept - which is
	// itself worth knowing years later.
	BackupPath string `json:"backup_path,omitempty"`
	// Outcome is "applied", "skipped", "rolled_back" or "failed", and Error
	// carries the reason for the last two.
	Outcome string `json:"outcome"`
	Error   string `json:"error,omitempty"`
}

// Reversible reports whether this entry carries enough to put the file back
// WITHOUT a copy of the original. That is the whole bargain: no automatic
// duplication of multi-gigabyte files, but every transformation written down
// precisely enough to be replayed backwards.
//
//   - audio_retime moved tracks by a known amount; applying the opposite shift
//     restores them exactly, and losslessly.
//   - container_fix renamed a file; renaming it back is the inverse.
//   - reindex rebuilt an index and left the picture and sound untouched, so
//     there is nothing a viewer would want undone.
//   - resync_damaged DISCARDED bytes. No record can bring them back, which is
//     why that one repair still demands a real backup (ErrBackupRequired).
func (r RepairRecord) Reversible() bool {
	if r.Outcome != "applied" {
		return false
	}
	switch r.Op {
	case RepairAudioRetime:
		return len(r.ShiftSec) > 0
	case RepairContainerFix:
		return true
	case RepairResync:
		return r.BackupPath != ""
	case RepairReindex, RepairRollback:
		return false
	}
	return false
}

// InverseShift is the shift that undoes this entry, per track. Empty when the
// entry is not a retime.
func (r RepairRecord) InverseShift() map[uint64]float64 {
	if r.Op != RepairAudioRetime || len(r.ShiftSec) == 0 {
		return nil
	}
	out := make(map[uint64]float64, len(r.ShiftSec))
	for track, sec := range r.ShiftSec {
		out[track] = -sec
	}
	return out
}

// RepairJournal records what a repair did. Implemented by the audit-log
// adapter; optional (nil disables journalling), because a journal failure must
// never abort a repair - but its absence is a defect to fix, not a mode to
// live in.
type RepairJournal interface {
	RecordRepair(ctx context.Context, rec RepairRecord) error
	// RepairHistory returns what was done to one file, newest first. This is
	// the read side that makes software rollback possible at all: without it
	// the entries are a write-only archive, which is no better than the mtime
	// forensics the 2026-07-13 incident had to settle for.
	RepairHistory(ctx context.Context, path string, limit int) ([]RepairRecord, error)
}

// TrackRetimer applies a signed per-track shift on the container timeline,
// losslessly and whatever the codec. It is the same primitive that caused the
// July damage, which is precisely why it is the one that can undo it: the
// inverse of a known shift is exact. Satisfied by the mkvgo adapter; optional
// (nil means a retime cannot be undone in software, and the API says so rather
// than pretending).
type TrackRetimer interface {
	RetimeTracks(ctx context.Context, path string, shiftSec map[uint64]float64) error
}

// WithTrackRetimer wires the primitive that replays a repair backwards.
func (s *SeekIndexService) WithTrackRetimer(r TrackRetimer) *SeekIndexService {
	s.retimer = r
	return s
}

// WithRepairJournal wires the journal. Optional and chainable.
func (s *SeekIndexService) WithRepairJournal(j RepairJournal) *SeekIndexService {
	s.journal = j
	return s
}

// journalRepair writes one entry, best effort. A journal that is down must not
// cost the operator a repair, so the failure is logged and swallowed - but it
// is logged at ERROR, because a rewrite nobody recorded is how the July
// incident became unexplainable.
func (s *SeekIndexService) journalRepair(ctx context.Context, rec RepairRecord) {
	if s.journal == nil {
		return
	}
	rec.Schema = RepairJournalSchema
	if err := s.journal.RecordRepair(ctx, rec); err != nil {
		s.logger.Error("seek-index: the repair journal refused an entry - this rewrite is now untraceable",
			"path", rec.Path, "op", string(rec.Op), "err", err)
	}
}
