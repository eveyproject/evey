package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ReinterleaveResult is what a re-mux leaves behind.
type ReinterleaveResult struct {
	BackupPath string // the original, kept as <path>.mkvgobak
}

// Reinterleaver re-muxes a file whose tracks sit in blocks of one track at a
// time, proving the output carries the same packets before swapping it in.
// Satisfied by the ffmpeg adapter; optional.
type Reinterleaver interface {
	Reinterleave(ctx context.Context, path string) (ReinterleaveResult, error)
}

// ErrReinterleaveUnavailable: no reinterleaver wired.
var ErrReinterleaveUnavailable = errors.New("seek-index: reinterleave not available on this server")

// ErrStillChunked: the rewrite went through but the probe still reads the
// tracks in blocks - the original is put back.
var ErrStillChunked = errors.New("seek-index: the re-mux still reads as one-track blocks")

// ErrReinterleaveUnverified: the rewrite went through but the probe that
// should confirm the layout could not run. A verification that failed is not
// a verification that passed: the original is put back.
var ErrReinterleaveUnverified = errors.New("seek-index: the re-mux could not be verified")

// ErrDiagnosisStale: the file changed since its diagnosis and could not be
// re-diagnosed - nothing acts on the old verdict.
var ErrDiagnosisStale = errors.New("seek-index: the diagnosis is stale and the file could not be re-diagnosed")

// ErrCatalogNotUpdated: the file is re-muxed and verified, but the catalog
// still carries the playback refusal - a scan re-records the measure.
var ErrCatalogNotUpdated = errors.New("seek-index: file re-muxed and verified, but the catalog could not be updated - run a scan to lift the playback refusal")

// WithReinterleaver wires the re-mux repair of chunked files.
func (s *SeekIndexService) WithReinterleaver(r Reinterleaver) *SeekIndexService {
	s.reinterleaver = r
	return s
}

// SupportsReinterleave reports whether the re-mux repair is wired.
func (s *SeekIndexService) SupportsReinterleave() bool { return s.reinterleaver != nil }

// StartReinterleaveMedia re-muxes one chunked file in the background. The
// file must carry the BrokenInterleave verdict in a cached scan.
func (s *SeekIndexService) StartReinterleaveMedia(ctx context.Context, mediaID string) error {
	if s.reinterleaver == nil {
		return ErrReinterleaveUnavailable
	}
	ref, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("seek-index: media %s: %w", mediaID, err)
	}
	st, ok := s.cachedStatusByPath(ref.Path)
	if !ok || !st.BrokenInterleave() {
		return fmt.Errorf("seek-index: %s is not diagnosed as stored in blocks: %w", ref.Path, ErrAckNotApplicable)
	}
	return s.startSingle(ctx, JobReinterleave, mediaID, func(bg context.Context, path string) (bool, error) {
		return s.reinterleaveStep(bg, path, nil)
	})
}

// reinterleaveStep re-muxes one file with the same guards as a reindex:
// journal opened before and closed whatever happens, audible tracks counted
// before and after (a loss restores the backup), the probe re-run on the
// result, then the catalog told the file is playable again.
func (s *SeekIndexService) reinterleaveStep(bg context.Context, path string, rb *rollingBackups) (bool, error) {
	st, ok := s.cachedStatusByPath(path)
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrNotInScan, path)
	}
	if !st.BrokenInterleave() {
		return true, nil
	}
	rec := RepairRecord{
		Path:       path,
		MediaID:    st.MediaID,
		Op:         RepairReinterleave,
		Job:        s.runningJobKind(),
		Started:    time.Now(),
		SizeBefore: fileSizeOf(path),
		Evidence:   RepairEvidence{Method: "interleave-probe", Detail: fmt.Sprintf("cluster timestamps step back %.0f s", st.InterleaveBackstepSec)},
	}
	audibleBefore, haveBaseline := s.audibleTracks(bg, path)
	rec.AudibleBefore = audibleBefore

	res, err := s.reinterleaver.Reinterleave(bg, path)
	if err != nil {
		rec.Outcome, rec.Error = "failed", err.Error()
		s.journalRepair(bg, rec)
		return false, err
	}
	rec.BackupPath = res.BackupPath
	restore := func() error { return s.reindexer.RestoreBackup(bg, path) }
	if haveBaseline {
		after, ok := s.audibleTracks(bg, path)
		rec.AudibleAfter = after
		if err := s.guardAudioSurvived(bg, path, audibleBefore, after, ok, res.BackupPath != ""); err != nil {
			rec.Outcome, rec.Error = repairOutcomeAfterGuard(err), err.Error()
			if rec.Outcome == "rolled_back" {
				rec.BackupPath = ""
			}
			s.journalRepair(bg, rec)
			return false, err
		}
	}
	if s.interleave != nil {
		// Fail-closed: a probe that errors is not a probe that passed. The
		// re-mux is proven lossless, so putting the original back costs
		// nothing but the retry - declaring the file healthy on no evidence
		// would hand playback a file nobody measured.
		back, perr := s.interleave.ProbeInterleaveBackstepSec(bg, path)
		var verr error
		switch {
		case perr != nil:
			verr = fmt.Errorf("%w: %w", ErrReinterleaveUnverified, perr)
		case back > InterleaveBackstepFlagSec:
			verr = fmt.Errorf("%w (%.0f s)", ErrStillChunked, back)
		}
		if verr != nil {
			if res.BackupPath != "" && restore() == nil {
				rec.Outcome, rec.BackupPath = "rolled_back", ""
			} else {
				rec.Outcome = "failed"
			}
			rec.Error = verr.Error()
			s.journalRepair(bg, rec)
			return false, verr
		}
	}
	s.refreshFileState(bg, path)
	// The cache reflects the file (re-muxed, verified) whatever the catalog
	// says: a stale cache would send the next batch re-muxing a sound file.
	// The catalog failure is surfaced as this file's failure instead, and a
	// scan - which re-probes the changed fingerprint - re-records the measure.
	catErr := s.recordInterleave(bg, path, 0)
	hasBak := res.BackupPath != ""
	s.updateCachedFile(path, func(c *SeekStatus) {
		c.InterleaveBackstepSec = 0
		c.HasBackup = hasBak
		c.DiagVersion = SeekDiagVersion
	})
	s.saveScans()
	if hasBak && rb != nil && rb.max > 0 && !rb.keepAll {
		rb.window = append(rb.window, path)
		if len(rb.window) > rb.max {
			s.purgeRolledBackup(bg, rb.window[0])
			rb.window = rb.window[1:]
		}
	}
	rec.Outcome = "applied"
	rec.SizeAfter = fileSizeOf(path)
	if catErr != nil {
		catErr = fmt.Errorf("%w: %w", ErrCatalogNotUpdated, catErr)
		rec.Error = catErr.Error()
	}
	s.journalRepair(bg, rec)
	s.logger.Info("seek-index: tracks re-interleaved", slog.String("path", path), slog.Bool("backup", hasBak))
	return false, catErr
}
