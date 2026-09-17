package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// seek_index_job.go holds the single-slot job runner: lifecycle
// (tryStart/finish), cancellation, idle-playback yielding, progress state and
// the batch/single orchestration every Start* entry point drives.

// Cancel stops the running job at the next file boundary and reports whether
// there was one to stop. Files are never left half-written: each repair writes
// to a temp file and swaps atomically, so cancellation lands between files, not
// inside one.
//
// Before this existed, "Repair all" on a large library could not be stopped by
// anything - not the API, not SIGTERM, which the admin restart path sends. An
// operator who started a multi-hour rewrite by mistake had to wait it out.
func (s *SeekIndexService) Cancel() bool {
	s.mu.Lock()
	cancel := s.cancel
	if cancel != nil {
		s.cancelled = true
	}
	s.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// stopWait bounds Stop: with the context cancelled, ffmpeg is killed at once
// and a rewrite gives up at its next check, so a job that has not finished
// within this is one nothing here can hurry - the process exits over it.
const stopWait = 15 * time.Second

// Stop is the shutdown hook: cancel whatever job runs and wait for it to
// finish - journal entry written, marker cleared, cache saved - before the
// database closes under it. Returns at once when nothing runs.
func (s *SeekIndexService) Stop() {
	if !s.Cancel() {
		return
	}
	deadline := time.Now().Add(stopWait)
	for time.Now().Before(deadline) {
		if !s.Status().Running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.logger.Warn("seek-index: job still running at shutdown, leaving it (marker stays for the restart)")
}

// waitForIdlePlayback blocks until nothing is being watched, or until the job
// is cancelled. No checker wired means never wait.
func (s *SeekIndexService) waitForIdlePlayback(ctx context.Context) {
	if s.activeStreams == nil {
		return
	}
	poll := s.idlePoll
	if poll <= 0 {
		poll = defaultIdlePoll
	}
	for s.activeStreams.ActivePlaybackCount() > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(poll):
		}
	}
}

// Status returns a copy of the current job state plus the last finished job.
func (s *SeekIndexService) Status() JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st JobStatus
	if s.running {
		st = s.cur
	}
	st.Running = s.running
	if s.last != nil {
		cp := *s.last
		cp.Failures = append([]Failure(nil), s.last.Failures...)
		st.Last = &cp
	}
	return st
}

// recoverJob is deferred by every job goroutine: a panic anywhere in the
// reindexer/catalog would otherwise leave the single job slot taken forever
// (every later start refused with ErrJobRunning until restart). When the job
// already finished cleanly, finish() was called and the recover is a no-op
// guard against double-finish (finish is only reached here on panic).
func (s *SeekIndexService) recoverJob() {
	if r := recover(); r != nil {
		s.logger.Error("seek-index job panicked", slog.Any("panic", r))
		s.finish(fmt.Errorf("job panicked: %v", r))
	}
}

func (s *SeekIndexService) startSingle(ctx context.Context, kind, mediaID string, fn func(context.Context, string) (bool, error)) error {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("seek-index %s: lookup media %s: %w", kind, mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("seek-index %s: media %s has no file", kind, mediaID)
	}
	bg, ok := s.tryStart(kind, "")
	if !ok {
		return ErrJobRunning
	}
	s.setPhase(PhaseProcessing, 1)
	go func() {
		defer s.recoverJob()
		s.processFiles(bg, kind, []string{f.Path}, fn)
		s.finish(nil)
	}()
	return nil
}

// startBatch runs scan → optional preflight → per-file processing → optional
// epilogue as one job. The preflight sees the kept candidates (with sizes)
// after the scan and can refuse the whole batch before a single file is
// touched; its error becomes the job-level error of the summary. The epilogue
// runs after the last file, still inside the job slot (e.g. the rolling
// backup window's end-of-job drain).
func (s *SeekIndexService) startBatch(kind, libraryID string, preflight func([]SeekStatus) error, keep func(SeekStatus) bool, fn func(context.Context, string) (bool, error), epilogue func(context.Context)) error {
	bg, ok := s.tryStart(kind, libraryID)
	if !ok {
		return ErrJobRunning
	}
	go func() {
		defer s.recoverJob()
		// Phase 1: reuse the cached scan when one exists - a full re-walk is
		// minutes of per-file probes and every repair re-checks its own file
		// before acting: the reindexer re-reads the index (an indexed file is
		// Skipped), the cache-driven repairs go through freshStatusForRepair
		// (a file replaced since the scan is re-diagnosed first, a vanished
		// one fails cleanly). The scan job itself always walks fresh.
		scan, _, ok := s.CachedScan(libraryID)
		if !ok {
			var err error
			scan, err = s.scanAndCache(bg, libraryID)
			if err != nil {
				s.finish(err)
				return
			}
		}
		var kept []SeekStatus
		for _, it := range scan.Items {
			if keep(it) {
				kept = append(kept, it)
			}
		}
		s.setPhase(PhaseProcessing, len(kept))
		if preflight != nil {
			if err := preflight(kept); err != nil {
				s.finish(err)
				return
			}
		}
		todo := make([]string, 0, len(kept))
		for _, it := range kept {
			todo = append(todo, it.Path)
		}
		s.processFiles(bg, kind, todo, fn)
		if epilogue != nil {
			epilogue(bg)
		}
		s.finish(nil)
	}()
	return nil
}

func (s *SeekIndexService) processFiles(bg context.Context, op string, paths []string, fn func(context.Context, string) (bool, error)) {
	for _, p := range paths {
		// Between files, not inside one: each repair writes a temp file and
		// swaps atomically, so stopping here can never leave a half-written
		// container. This loop used to run to completion no matter what - it
		// ignored cancellation entirely and never looked at whether anyone was
		// watching, while rewriting multi-GB files on the library disk.
		if bg.Err() != nil {
			s.logger.Info("seek-index "+op+" cancelled", slog.Int("remaining", len(paths)))
			return
		}
		s.waitForIdlePlayback(bg)
		if bg.Err() != nil {
			s.logger.Info("seek-index " + op + " cancelled while waiting for idle playback")
			return
		}
		s.setCurrent(p)
		skipped, err := fn(bg, p)
		if err != nil {
			s.recordFailure(p, err)
			s.logger.Warn("seek-index "+op+" failed (file left intact)",
				slog.String("path", p), slog.Any("err", err))
			continue
		}
		s.recordDone(skipped)
	}
}

// tryStart takes the single job slot and returns the context the job runs
// under: detached from the caller's request (the job outlives it), a child of
// the server's lifetime, and cancellable. Born under the lock with the slot, so
// there is no instant where a job runs and Cancel finds nothing to cancel.
func (s *SeekIndexService) tryStart(kind, libraryID string) (context.Context, bool) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, false
	}
	ctx, cancel := context.WithCancel(s.baseCtx)
	s.jobCtx, s.cancel, s.cancelled = ctx, cancel, false
	s.running = true
	s.cur = JobStatus{
		Kind:      kind,
		Scope:     libraryID,
		Phase:     PhaseScanning,
		StartedAt: time.Now(),
	}
	data := s.marshalJobMarker()
	s.mu.Unlock()
	s.writeJobMarker(data)
	return ctx, true
}

func (s *SeekIndexService) setPhase(phase string, total int) {
	s.mu.Lock()
	s.cur.Phase = phase
	s.cur.Total = total
	s.cur.Done, s.cur.Failed, s.cur.Skipped = 0, 0, 0
	s.cur.CurrentPath = ""
	data := s.marshalJobMarker()
	s.mu.Unlock()
	s.writeJobMarker(data)
}

func (s *SeekIndexService) setCurrent(path string) {
	s.mu.Lock()
	s.cur.CurrentPath = path
	s.mu.Unlock()
}

func (s *SeekIndexService) recordDone(skipped bool) {
	s.mu.Lock()
	s.cur.Done++
	if skipped {
		s.cur.Skipped++
	}
	data := s.marshalJobMarker()
	s.mu.Unlock()
	s.writeJobMarker(data)
}

// jobFailures accumulates outside JobStatus so Status() copies stay cheap.
type jobFailures struct {
	list     []Failure
	readOnly bool
}

func (s *SeekIndexService) recordFailure(path string, err error) {
	s.mu.Lock()
	s.cur.Done++
	s.cur.Failed++
	ro := IsReadOnlyFS(err)
	if ro {
		s.curFailures.readOnly = true
	}
	if len(s.curFailures.list) < maxFailures {
		s.curFailures.list = append(s.curFailures.list, Failure{Path: path, Error: err.Error(), ReadOnly: ro})
	}
	data := s.marshalJobMarker()
	s.mu.Unlock()
	s.writeJobMarker(data)
}

func (s *SeekIndexService) finish(jobErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Cancelled by the operator (Cancel/Stop) or by the server going down (the
	// base context): read BEFORE releasing the context below, which cancels it
	// in every case.
	cancelled := s.cancelled || (s.jobCtx != nil && s.jobCtx.Err() != nil)
	// Release the job's context: leaving the CancelFunc live would leak it and
	// let a late Cancel() apply to whatever job starts next.
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.jobCtx = nil
	sum := &JobSummary{
		Kind:       s.cur.Kind,
		Scope:      s.cur.Scope,
		Done:       s.cur.Done,
		Failed:     s.cur.Failed,
		Skipped:    s.cur.Skipped,
		Total:      s.cur.Total,
		FinishedAt: time.Now(),
		ReadOnly:   s.curFailures.readOnly,
		Failures:   s.curFailures.list,
		Cancelled:  cancelled,
	}
	if jobErr != nil {
		sum.Err = jobErr.Error()
		if IsReadOnlyFS(jobErr) {
			sum.ReadOnly = true
		}
	}
	// Persist BEFORE releasing the job slot: the marker is consumed (the job
	// ended cleanly) and the cache snapshot reflects the repairs. Callers that
	// wait for Running=false may tear the state dir down right after - the
	// writes must already be behind us. saveScans takes the lock itself.
	s.mu.Unlock()
	s.clearJobMarker()
	s.saveScans()
	s.mu.Lock()
	s.last = sum
	s.running = false
	s.cur = JobStatus{}
	s.curFailures = jobFailures{}
	s.logger.Info("seek-index job finished",
		slog.String("kind", sum.Kind), slog.String("scope", sum.Scope),
		slog.Int("done", sum.Done), slog.Int("failed", sum.Failed),
		slog.Int("skipped", sum.Skipped), slog.Int("total", sum.Total),
		slog.Bool("cancelled", sum.Cancelled), slog.String("err", sum.Err))
}
