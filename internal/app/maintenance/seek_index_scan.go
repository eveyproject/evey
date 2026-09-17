package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
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
// run is variadic so the ~30 existing call sites (and every caller that has no
// opinion on the pace) keep the two-argument form and the gentle default.
func (s *SeekIndexService) StartScan(_ context.Context, libraryID string, run ...ScanRunOptions) error {
	if !s.tryStart(JobScan, libraryID) {
		return ErrJobRunning
	}
	var opts ScanRunOptions
	if len(run) > 0 {
		opts = run[0]
	}
	go func() {
		defer s.recoverJob()
		// Detached from the triggering request (the job outlives it) but
		// cancellable, so Cancel and shutdown can stop it.
		bg := s.jobContext()
		_, err := s.scanAndCachePaced(bg, libraryID, opts)
		s.finish(err)
	}()
	return nil
}

// scanAndCache walks the scope at the gentle pace - the one every caller that
// has no operator choice to relay wants (a repair batch's phase 1 already
// competes with its own rewrites for the disk).
func (s *SeekIndexService) scanAndCache(ctx context.Context, libraryID string) (ScanResult, error) {
	return s.scanAndCachePaced(ctx, libraryID, ScanRunOptions{})
}

// scanAndCachePaced walks the scope at the operator's chosen pace, publishing
// progress, and stores the result for CachedScan.
func (s *SeekIndexService) scanAndCachePaced(ctx context.Context, libraryID string, run ScanRunOptions) (ScanResult, error) {
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
	// One slot per candidate, filled in place: whichever pace ran, the rollup
	// below folds the files in catalog order, so the two paces cannot disagree
	// on what the library looks like - only on how long it took to find out.
	probed := make([]probeOutcome, len(candidates))
	if run.pace() == ScanPaceFast {
		s.probeInParallel(ctx, candidates, prev, probed)
	} else {
		s.probeGently(ctx, candidates, prev, probed)
	}

	reused := 0
	var res ScanResult
	for _, p := range probed {
		if !p.ok {
			continue
		}
		if p.reused {
			reused++
		}
		res.Items = append(res.Items, p.st)
		res.Total++
		addToRollups(&res, p.st)
	}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if reused > 0 {
		s.logger.Info("seek-index scan: reused unchanged diagnoses",
			slog.Int("reused", reused), slog.Int("total", res.Total))
	}

	s.mu.Lock()
	// The statuses above were reused from a snapshot taken when the scan
	// started; an acknowledgement the operator posted since then lives only
	// in the CURRENT cache and would be discarded by the write below. Carry
	// it over for every unchanged file before replacing the entry.
	s.carryAcksLocked(&res)
	s.scans[jobKey(libraryID)] = cachedScan{res: res, at: time.Now()}
	s.mu.Unlock()
	s.saveScans()
	return res, nil
}

// probeOutcome is one candidate's slot in the scan's result table. Filled in
// place (never appended) so a parallel walk keeps the catalog's order.
type probeOutcome struct {
	st     SeekStatus
	reused bool
	ok     bool
}

// probeGently walks the candidates one at a time and, after every file it
// actually read, stands aside for as long as that read took. The scan's cost is
// not CPU but head movement - three zones per file (head, cues near the end,
// tail) on disks a player is trying to read at the same time - and there is no
// portable ionice from a container. Yielding half the wall clock is the lever
// that exists: measured against the file just probed, so it scales itself.
//
// A file whose diagnosis was REUSED read nothing and owes nothing back:
// pausing for it would double a scan that costs almost nothing (between two
// scans virtually no file changed).
func (s *SeekIndexService) probeGently(ctx context.Context, candidates []MediaRef, prev map[string]SeekStatus, out []probeOutcome) {
	for i, f := range candidates {
		if ctx.Err() != nil {
			return
		}
		s.setCurrent(f.Path)
		start := s.now()
		st, reused, ok := s.probeGuarded(ctx, f, prev)
		s.recordDone(false)
		out[i] = probeOutcome{st: st, reused: reused, ok: ok}
		if reused {
			continue
		}
		if d := time.Duration(float64(s.now().Sub(start)) * scanGentleIdleRatio); d > 0 {
			s.sleep(ctx, d)
		}
	}
}

// probeInParallel spreads the walk over a bounded pool. The library lives on a
// pool of several disks (mergerfs over three ext4 volumes in prod), and one
// file at a time keeps exactly one spindle busy: the first full scan of 6695
// files takes ~2h that way. No pause here - this pace is the operator saying
// they want the disks for the scan.
func (s *SeekIndexService) probeInParallel(ctx context.Context, candidates []MediaRef, prev map[string]SeekStatus, out []probeOutcome) {
	workers := min(scanFastWorkers, len(candidates))
	if workers <= 0 {
		return
	}
	feed := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range feed {
				if ctx.Err() != nil {
					return
				}
				f := candidates[i]
				s.setCurrent(f.Path)
				st, reused, ok := s.probeGuarded(ctx, f, prev)
				s.recordDone(false)
				// Disjoint indices, so no lock: the slots are read only after
				// wg.Wait, which is the happens-before edge.
				out[i] = probeOutcome{st: st, reused: reused, ok: ok}
			}
		})
	}
	// The send must watch the cancellation too: the workers leave the moment
	// the job is cancelled, and a bare send into a channel nobody reads any
	// more would park this goroutine for good - the job would never finish.
feeding:
	for i := range candidates {
		select {
		case feed <- i:
		case <-ctx.Done():
			break feeding
		}
	}
	close(feed)
	wg.Wait()
}

// probeGuarded turns a panic in a prober into one skipped file. The job
// goroutine has recoverJob; a worker goroutine has nothing, and a panic
// escaping there takes the whole server down instead of the scan.
func (s *SeekIndexService) probeGuarded(ctx context.Context, f MediaRef, prev map[string]SeekStatus) (st SeekStatus, reused, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("seek-index scan: probe panicked, skipping file",
				slog.String("path", f.Path), slog.Any("panic", r))
			st, reused, ok = SeekStatus{}, false, false
		}
	}()
	return s.probeFile(ctx, f, prev)
}

// carryAcksLocked copies the acknowledgement stamp of every cached status
// (any scope) onto the matching item of res when the file is the same bytes
// under the same detector version and the verdict still admits an ack - a
// file that changed or became repairable starts afresh. Rollups are rebuilt
// afterwards so the acknowledged count matches the items. Caller holds s.mu.
func (s *SeekIndexService) carryAcksLocked(res *ScanResult) {
	acked := make(map[string]SeekStatus)
	for _, c := range s.scans {
		for _, it := range c.res.Items {
			if it.Acked() {
				acked[it.Path] = it
			}
		}
	}
	if len(acked) == 0 {
		return
	}
	carried := 0
	for i := range res.Items {
		it := &res.Items[i]
		if it.Acked() {
			continue
		}
		cur, ok := acked[it.Path]
		if !ok || cur.SizeBytes != it.SizeBytes || cur.ModTimeUnix != it.ModTimeUnix || cur.DiagVersion != it.DiagVersion || !it.Ackable() {
			continue
		}
		it.AckedAt = cur.AckedAt
		carried++
	}
	if carried > 0 {
		rebuildRollups(res)
	}
}

// rebuildRollups recomputes every counter of res from its items.
func rebuildRollups(res *ScanResult) {
	fresh := ScanResult{Items: res.Items}
	for _, st := range fresh.Items {
		fresh.Total++
		addToRollups(&fresh, st)
	}
	*res = fresh
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
	if p, hit := prev[f.Path]; hit && p.ModTimeUnix != 0 && p.DiagVersion == SeekDiagVersion &&
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
			DiagVersion: SeekDiagVersion,
		}
		if !s.diagnose(ctx, f.Path, &st) {
			return SeekStatus{}, false, false
		}
		s.probeInterleave(ctx, f.Path, &st)
		s.proveAudioCoherence(ctx, f.Path, &st)
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
		if st.IndexSparse {
			st.IndexSparseDetail = d.IndexSparseDetail()
			st.PictureMissing = d.IndexSparsePictureMissing()
		}
		st.AudioDelaySec = float64(d.MaxAudioDelayMs()) / 1000
		// Every audio track's own offset, not just the largest: the count
		// decides whether the coherence comparison has anything to compare,
		// and the SHAPE of the set is the fingerprint of a retimed track.
		st.AudioTracks = len(d.AudioDelaysMs)
		st.AudioTrackZeroed = !st.WrongContainer && RetimeSignature(d.AudioDelaysMs)
		st.Truncated = d.Truncated
		st.MissingTailBytes = d.MissingTailBytes
		st.DamagedBytes = d.DamagedBytes
		st.TrailingJunk = d.TrailingJunk()
		// How far before the VIDEO's end the audio content stops (mkvgo
		// TrackEnds). Meaningless for a wrong-container file (its Matroska
		// duration is not one) and redundant for a truncated one (the WHOLE
		// tail is missing - the shortfall would only restate it). Gaps under
		// AudioShortfallFlagSec are ordinary mux slack (priming, final-cluster
		// rounding) and stay unreported so the flag means what the UI says.
		if !st.WrongContainer && !st.Truncated {
			if gap := float64(d.AudioShortfallMs) / 1000; gap > AudioShortfallFlagSec {
				st.AudioShortfallSec = gap
			}
		}
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

// probeInterleave fills InterleaveBackstepSec. Secondary diagnostics: a probe
// failure logs and leaves the file in the scan with no finding.
func (s *SeekIndexService) probeInterleave(ctx context.Context, path string, st *SeekStatus) {
	if s.interleave == nil || st.WrongContainer {
		return
	}
	back, err := s.interleave.ProbeInterleaveBackstepSec(ctx, path)
	if err != nil {
		s.logger.Warn("seek-index scan: interleave probe failed",
			slog.String("path", path), slog.Any("err", err))
		return
	}
	if back > InterleaveBackstepFlagSec {
		st.InterleaveBackstepSec = back
	}
	s.recordInterleave(ctx, path, back)
}

// recordInterleave pushes the measure to the catalog (0 included: a repaired
// or re-acquired file must lose its refusal).
func (s *SeekIndexService) recordInterleave(ctx context.Context, path string, backstepSec float64) {
	if s.interleaveRec == nil {
		return
	}
	if err := s.interleaveRec.SetInterleaveBackstep(ctx, path, int64(backstepSec*1000)); err != nil {
		s.logger.Warn("seek-index scan: interleave record failed",
			slog.String("path", path), slog.Any("err", err))
	}
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
	case st.SeekCoarse(), st.PictureMissing:
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
	if st.BrokenInterleave() {
		res.BrokenInterleave++
	}
	switch v := st.Verdict(); {
	case v == VerdictRepairable:
		res.Repairable++
	case st.Acked():
		res.Acked++
	case v == VerdictInformational:
		res.Informational++
	}
}

// recomputeRollups rebuilds every rollup counter from the items.
func recomputeRollups(res *ScanResult) {
	res.MissingIndex, res.WithBackup, res.AudioDelayed = 0, 0, 0
	res.Truncated, res.Damaged, res.WrongContainer = 0, 0, 0
	res.TrailingJunk, res.AudioShort, res.SeekCoarse = 0, 0, 0
	res.BrokenInterleave = 0
	res.Repairable, res.Informational, res.Acked = 0, 0, 0
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
			st.HasIndex, st.IndexSparse, st.IndexSparseDetail = true, false, ""
			st.DamagedBytes, st.Truncated, st.TrailingJunk = 0, false, false
			st.ResyncRequired = false
			return
		}
		st.DiagVersion = SeekDiagVersion
		// The rewrite went through, so whatever refused it before is gone from
		// the file: a stale ResyncRequired would keep a repaired file pinned in
		// the resync bucket for good.
		st.ResyncRequired = false
		st.HasIndex, st.IndexSparse, st.IndexSparseDetail = fresh.HasIndex, fresh.IndexSparse, fresh.IndexSparseDetail
		st.PictureMissing = fresh.PictureMissing
		st.WrongContainer = fresh.WrongContainer
		st.Truncated, st.MissingTailBytes = fresh.Truncated, fresh.MissingTailBytes
		st.DamagedBytes, st.TrailingJunk = fresh.DamagedBytes, fresh.TrailingJunk
		st.AudioDelaySec = fresh.AudioDelaySec
	})
}

// proveAudioCoherence asks the one question that tells a sound file from a
// damaged one: do its own audio tracks tell the same story at the same
// instant? It runs on EVERY file with two or more audio tracks.
//
// It used to run only on files DECLARING an offset above the floor - 3 of 6695
// on the real library - on the theory that the offset was the thing to
// explain. That theory was backwards. A track that has been pulled back onto
// the picture declares NO offset any more: that is precisely what the repair
// did to it. a prod title declared 0.000 s and another 0.02 s after the
// 2026-07-13 batch, and both were classed healthy; none of the 98 rewritten
// files would ever have been looked at. The cost is about two seconds per
// multi-track file (the video is not demuxed), which the gentle scan pace
// absorbs.
//
// A file whose own audio tracks line up is a file whose offsets are RIGHT: a
// dub aligned to its cut, which nothing here should ever rewrite. A file whose
// tracks disagree has one track in the wrong place, and since nothing here can
// tell which, it is reported and nothing more - never an automatic repair. A
// measurement too weak to mean anything (different mixes, a commentary track)
// changes nothing at all: silence is not an accusation.
func (s *SeekIndexService) proveAudioCoherence(ctx context.Context, path string, st *SeekStatus) {
	if s.audioCoh == nil || st.WrongContainer || st.Truncated {
		return
	}
	// Without a track count (no diagnoser) the old trigger stands: an offset
	// worth explaining. With one, a single-track file has nothing to compare
	// and is not worth a subprocess.
	if st.AudioTracks == 0 && st.AudioDelaySec <= AudioDelayFlagSec || st.AudioTracks == 1 {
		return
	}
	lag, confidence, err := s.audioCoh.ProbeAudioTrackLagSec(ctx, path)
	if err != nil {
		s.logger.Warn("seek-index scan: audio coherence probe failed",
			slog.String("path", path), slog.Any("err", err))
		return
	}
	if confidence < AudioCoherenceMinConfidence {
		// Nothing to compare, or nothing legible in the comparison. The offset
		// stays what it was: a fact about the file, reported as such.
		return
	}
	st.AudioTracksChecked = true
	if lag < 0 {
		lag = -lag
	}
	st.AudioTracksLagSec = lag
}
