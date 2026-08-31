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

	prev := s.previousStatuses()
	reused := 0
	var res ScanResult
	for _, f := range candidates {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		s.setCurrent(f.Path)
		st, wasReused, ok := s.probeFile(ctx, f, prev)
		s.recordDone(false)
		if !ok {
			continue
		}
		if wasReused {
			reused++
		}
		res.Items = append(res.Items, st)
		res.Total++
		addToRollups(&res, st)
	}
	if reused > 0 {
		s.logger.Info("seek-index scan: reused unchanged diagnoses",
			slog.Int("reused", reused), slog.Int("total", res.Total))
	}

	s.mu.Lock()
	s.scans[jobKey(libraryID)] = cachedScan{res: res, at: time.Now()}
	s.mu.Unlock()
	s.saveScans()
	return res, nil
}

// previousStatuses snapshots every cached SeekStatus by path (all scopes) so
// an incremental re-scan can reuse the diagnosis of unchanged files.
func (s *SeekIndexService) previousStatuses() map[string]SeekStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := make(map[string]SeekStatus)
	for _, c := range s.scans {
		for _, it := range c.res.Items {
			prev[it.Path] = it
		}
	}
	return prev
}

func (s *SeekIndexService) probeFile(ctx context.Context, f MediaRef, prev map[string]SeekStatus) (st SeekStatus, reused, ok bool) {
	fi, statErr := os.Stat(f.Path)
	if statErr != nil {
		// File recorded in the DB but missing/unreadable on disk - skip it
		// rather than reporting a misleading "no index".
		s.logger.Warn("seek-index scan: file unreadable, skipping",
			slog.String("path", f.Path), slog.Any("err", statErr))
		return SeekStatus{}, false, false
	}

	// Incremental reuse: (size, mtime) unchanged since the last completed scan
	// → the file's bytes are the same, so its diagnosis is too. Only the
	// backup flag is live state (a rollback/purge changes it without touching
	// the media), so it is re-stat'ed below either way.
	if p, hit := prev[f.Path]; hit && p.ModTimeUnix != 0 &&
		p.SizeBytes == fi.Size() && p.ModTimeUnix == fi.ModTime().Unix() {
		st = p
		// Identity may have moved (rename in the DB) even when the file did not.
		st.MediaID, st.Title = f.MediaID, f.Title
		reused = true
	} else {
		st = SeekStatus{
			MediaID:     f.MediaID,
			Title:       f.Title,
			Path:        f.Path,
			Container:   containerOf(f.Path),
			SizeBytes:   fi.Size(),
			ModTimeUnix: fi.ModTime().Unix(),
		}
		if !s.diagnose(ctx, f.Path, &st) {
			return SeekStatus{}, false, false
		}
		s.probeAudioTail(ctx, f.Path, &st)
	}

	hasBak, bakErr := s.reindexer.HasBackup(ctx, f.Path)
	if bakErr != nil {
		// Backup status is secondary diagnostics - a probe failure must not
		// drop the file from the scan, just report it as "no backup".
		s.logger.Warn("seek-index scan: backup probe failed",
			slog.String("path", f.Path), slog.Any("err", bakErr))
	}
	st.HasBackup = hasBak
	return st, reused, true
}

// probeAudioTail fills AudioShortfallSec: how far before the VIDEO's end the
// audio content stops. Skipped for a wrong-container file (its Matroska
// duration is meaningless) and for a truncated one (the WHOLE tail is
// missing - the shortfall would only restate it). Secondary diagnostics: a
// probe failure logs and leaves 0 rather than dropping the file. Gaps under
// AudioShortfallFlagSec are ordinary mux slack (priming, final-cluster
// rounding) and stay unreported so the flag means what the UI says it means.
func (s *SeekIndexService) probeAudioTail(ctx context.Context, path string, st *SeekStatus) {
	if s.audioTail == nil || st.WrongContainer || st.Truncated {
		return
	}
	gap, err := s.audioTail.ProbeAudioTailGapSec(ctx, path)
	if err != nil {
		s.logger.Warn("seek-index scan: audio tail probe failed",
			slog.String("path", path), slog.Any("err", err))
		return
	}
	if gap > AudioShortfallFlagSec {
		st.AudioShortfallSec = gap
	}
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
		st.IndexSparse = !st.WrongContainer && d.IndexSparse()
		st.AudioDelaySec = float64(d.MaxAudioDelayMs()) / 1000
		st.Truncated = d.Truncated
		st.MissingTailBytes = d.MissingTailBytes
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

// addToRollups folds one file's status into the result's rollup counters. THE
// single rollup rule - the scan loop, the cached-file update and the persisted
// -scan reload all fold through here, so a new category cannot be counted in
// one path and silently read 0 in another (the persisted-scan reload used to
// recompute only three of seven counters after a restart).
func addToRollups(res *ScanResult, st SeekStatus) {
	// A rebuilt-and-still-coarse file has no usable index either, but counting
	// it as "missing" would put it back in a repair queue that can do nothing
	// for it. It gets its own bucket instead - see SeekStatus.SeekCoarse.
	switch {
	case st.SeekCoarse():
		res.SeekCoarse++
	case !st.HasIndex:
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
	if st.AudioShortfallSec > AudioShortfallFlagSec {
		res.AudioShort++
	}
	switch v := st.Verdict(); {
	case v == VerdictRepairable:
		res.Repairable++
	case st.Acked():
		res.Acked++
	case v == VerdictManual:
		res.Manual++
	case v == VerdictInformational:
		res.Informational++
	}
}

// recomputeRollups rebuilds every rollup counter from the items.
func recomputeRollups(res *ScanResult) {
	res.MissingIndex, res.WithBackup, res.AudioDelayed = 0, 0, 0
	res.Truncated, res.Damaged, res.WrongContainer = 0, 0, 0
	res.TrailingJunk, res.AudioShort, res.SeekCoarse = 0, 0, 0
	res.Repairable, res.Manual, res.Informational, res.Acked = 0, 0, 0, 0
	for _, it := range res.Items {
		addToRollups(res, it)
	}
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
		recomputeRollups(&c.res)
		s.scans[key] = c
	}
}

// noteReindexed folds a successful rewrite into the cached scan by RE-DIAGNOSING
// the file instead of declaring victory. A rewrite always produces an index, but
// not necessarily a usable one: an encode whose own keyframes sit a minute apart
// comes back "index-sparse" however faithfully it is rebuilt. Asserting
// HasIndex = true regardless showed the operator a repair that had not happened,
// and the next scan re-flagged the file for a repair that would skip it - the
// sterile loop again, one level up.
//
// Re-stamping (size, mtime) is what makes the verdict STICK: the incremental
// re-scan reuses a status only while that fingerprint holds, so ReindexAttempted
// survives exactly as long as the bytes we judged do. Re-acquiring the source
// clears it and the file gets a fresh chance, which is the behaviour we want.
//
// Best-effort by design: with no diagnoser wired (or an unreadable file) it
// keeps the historic optimistic answer rather than inventing a defect.
func (s *SeekIndexService) noteReindexed(ctx context.Context, path string, hasBak bool) {
	var fresh SeekStatus
	ok := s.diagnoser != nil && s.diagnose(ctx, path, &fresh)
	var size, mtime int64
	if fi, err := os.Stat(path); err == nil {
		size, mtime = fi.Size(), fi.ModTime().Unix()
	}
	s.updateCachedFile(path, func(st *SeekStatus) {
		st.HasBackup = hasBak
		st.ReindexAttempted = true
		if size > 0 {
			st.SizeBytes, st.ModTimeUnix = size, mtime
		}
		if !ok {
			st.HasIndex, st.IndexSparse = true, false
			st.DamagedBytes, st.Truncated, st.TrailingJunk = 0, false, false
			return
		}
		st.HasIndex, st.IndexSparse = fresh.HasIndex, fresh.IndexSparse
		st.WrongContainer = fresh.WrongContainer
		st.Truncated, st.MissingTailBytes = fresh.Truncated, fresh.MissingTailBytes
		st.DamagedBytes, st.TrailingJunk = fresh.DamagedBytes, fresh.TrailingJunk
		st.AudioDelaySec = fresh.AudioDelaySec
	})
}
