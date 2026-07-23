package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// seek_index_scan.go holds the library scan: per-file probe/diagnose, the
// rollup into ScanResult, the in-memory cache and its incremental update.

// CachedScan returns the last completed scan for the scope, if any. Instant -
// no disk I/O.
func (s *SeekIndexService) CachedScan(libraryID string) (ScanResult, time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.scans[jobKey(libraryID)]
	return c.res, c.at, ok
}

// StartScan kicks a background scan of the library (all libraries when empty)
// and returns immediately. Poll Status() for progress; fetch the result with
// CachedScan once the job finishes.
func (s *SeekIndexService) StartScan(_ context.Context, libraryID string) error {
	if !s.tryStart(JobScan, libraryID) {
		return ErrJobRunning
	}
	go func() {
		defer s.recoverJob()
		// Detached from the triggering request (the job outlives it) but
		// cancellable, so Cancel and shutdown can stop it.
		bg := s.jobContext()
		_, err := s.scanAndCache(bg, libraryID)
		s.finish(err)
	}()
	return nil
}

// scanAndCache walks the scope, publishing progress, and stores the result for
// CachedScan. Used both by scan jobs and as phase 1 of batch jobs.
func (s *SeekIndexService) scanAndCache(ctx context.Context, libraryID string) (ScanResult, error) {
	files, err := s.catalog.ListMediaFiles(ctx, libraryID)
	if err != nil {
		return ScanResult{}, fmt.Errorf("seek-index scan: list media: %w", err)
	}
	var candidates []MediaRef
	for _, f := range files {
		if f.Path != "" && isEBMLPath(f.Path) {
			candidates = append(candidates, f)
		}
	}
	s.setPhase(PhaseScanning, len(candidates))

	var res ScanResult
	for _, f := range candidates {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		s.setCurrent(f.Path)
		st, ok := s.probeFile(ctx, f)
		s.recordDone(false)
		if !ok {
			continue
		}
		res.Items = append(res.Items, st)
		res.Total++
		if !st.HasIndex {
			res.MissingIndex++
		}
		if st.HasBackup {
			res.WithBackup++
		}
		if st.AudioDelaySec > AudioDelayFlagSec {
			res.AudioDelayed++
		}
		if st.Truncated {
			res.Truncated++
		} else if st.DamagedBytes > 0 {
			res.Damaged++
		}
		if st.WrongContainer {
			res.WrongContainer++
		}
		if st.TrailingJunk {
			res.TrailingJunk++
		}
	}

	s.mu.Lock()
	s.scans[jobKey(libraryID)] = cachedScan{res: res, at: time.Now()}
	s.mu.Unlock()
	s.saveScans()
	return res, nil
}

func (s *SeekIndexService) probeFile(ctx context.Context, f MediaRef) (SeekStatus, bool) {
	fi, statErr := os.Stat(f.Path)
	if statErr != nil {
		// File recorded in the DB but missing/unreadable on disk - skip it
		// rather than reporting a misleading "no index".
		s.logger.Warn("seek-index scan: file unreadable, skipping",
			slog.String("path", f.Path), slog.Any("err", statErr))
		return SeekStatus{}, false
	}
	st := SeekStatus{
		MediaID:   f.MediaID,
		Title:     f.Title,
		Path:      f.Path,
		Container: containerOf(f.Path),
		SizeBytes: fi.Size(),
	}
	if !s.diagnose(ctx, f.Path, &st) {
		return SeekStatus{}, false
	}
	hasBak, bakErr := s.reindexer.HasBackup(ctx, f.Path)
	if bakErr != nil {
		// Backup status is secondary diagnostics - a probe failure must not
		// drop the file from the scan, just report it as "no backup".
		s.logger.Warn("seek-index scan: backup probe failed",
			slog.String("path", f.Path), slog.Any("err", bakErr))
	}
	st.HasBackup = hasBak
	return st, true
}

// diagnose fills the defect fields of st (HasIndex, AudioDelaySec, Truncated,
// DamagedBytes). With a Diagnoser wired one head-mostly Diagnose subsumes the
// HasSeekIndex + audio-delay probes and adds the truncated/damaged split;
// without one it falls back to the separate probes. Returns false only when
// the file cannot be parsed at all (skip it from the scan).
func (s *SeekIndexService) diagnose(ctx context.Context, path string, st *SeekStatus) bool {
	if s.diagnoser != nil {
		d, err := s.diagnoser.Diagnose(ctx, path)
		if err != nil {
			s.logger.Warn("seek-index scan: diagnose failed, skipping",
				slog.String("path", path), slog.Any("err", err))
			return false
		}
		st.WrongContainer = d.WrongContainer()
		// A mislabelled MP4 has no Matroska seek index to build and a reindex
		// would fail on it; treat it as "has index" so it never lands in the
		// reindex list, and surface it through its own WrongContainer flag instead.
		st.HasIndex = st.WrongContainer || !d.HasIndexDefect()
		st.AudioDelaySec = float64(d.MaxAudioDelayMs()) / 1000
		st.Truncated = d.Truncated
		st.DamagedBytes = d.DamagedBytes
		st.TrailingJunk = d.TrailingJunk()
		return true
	}

	hasIdx, err := s.reindexer.HasSeekIndex(ctx, path)
	if err != nil {
		s.logger.Warn("seek-index scan: parse failed, skipping",
			slog.String("path", path), slog.Any("err", err))
		return false
	}
	st.HasIndex = hasIdx
	if s.audioDelay != nil {
		d, dErr := s.audioDelay.ProbeMaxAudioStartDelaySec(ctx, path)
		if dErr != nil {
			// Secondary diagnostics: report "no delay" rather than dropping
			// the file from the scan.
			s.logger.Warn("seek-index scan: audio-delay probe failed",
				slog.String("path", path), slog.Any("err", dErr))
		} else {
			st.AudioDelaySec = d
		}
	}
	return true
}

// updateCachedFile mutates the entry for path in every cached scan and fixes
// the rollups, so the UI reflects repairs without a full re-scan.
func (s *SeekIndexService) updateCachedFile(path string, mut func(*SeekStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, c := range s.scans {
		changed := false
		for i := range c.res.Items {
			if c.res.Items[i].Path == path {
				mut(&c.res.Items[i])
				changed = true
			}
		}
		if !changed {
			continue
		}
		c.res.MissingIndex, c.res.WithBackup, c.res.AudioDelayed = 0, 0, 0
		c.res.Truncated, c.res.Damaged, c.res.WrongContainer = 0, 0, 0
		c.res.TrailingJunk = 0
		for _, it := range c.res.Items {
			if !it.HasIndex {
				c.res.MissingIndex++
			}
			if it.HasBackup {
				c.res.WithBackup++
			}
			if it.AudioDelaySec > AudioDelayFlagSec {
				c.res.AudioDelayed++
			}
			if it.Truncated {
				c.res.Truncated++
			} else if it.DamagedBytes > 0 {
				c.res.Damaged++
			}
			if it.WrongContainer {
				c.res.WrongContainer++
			}
			if it.TrailingJunk {
				c.res.TrailingJunk++
			}
		}
		s.scans[key] = c
	}
}
