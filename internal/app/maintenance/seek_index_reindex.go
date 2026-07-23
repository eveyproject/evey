package maintenance

import (
	"context"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// seek_index_reindex.go holds the Cues-rebuild + "repair all" paths: the
// operator's ReindexRunOptions, the per-file reindex step and the rolling
// backup window shared by reindex and combined repair.

// ReindexRunOptions carries the operator's repair choices from the API layer.
type ReindexRunOptions struct {
	// Mode: "auto" (patch when the layout allows, rewrite otherwise),
	// mkv.ModeInPlace (surgical patch ONLY), anything else = full rewrite.
	Mode string
	// DeepVerify proves every rebuilt cue lands on a real video keyframe
	// before committing (a full read per file).
	DeepVerify bool
	// SkipBackup drops the rewrite's .mkvgobak after a successful swap so a
	// batch never accumulates one persistent copy per repaired file - the
	// option for a nearly-full volume. Deep-verify is forced on in exchange.
	SkipBackup bool
	// BackupWindow (library batches only, requires backups on) keeps only the
	// N most recent repairs' .mkvgobak while the job runs: as it advances past
	// the window, the oldest backup is purged automatically, and the job's end
	// drains the remainder - the volume never holds more than N extra copies
	// and finishes with none. The window is a short abort-and-rollback horizon
	// for a job the operator is watching, not a persistent archive.
	// 0 keeps every backup (the historic behaviour).
	BackupWindow int
	// Resync repairs a file the strict reindex REFUSED for a corrupted region:
	// the tolerant rewrite skips the damaged span and resynchronises. Always
	// rewrites and forces a backup (this repair loses the skipped bytes). Used
	// for the per-file "repair anyway" action on a file that failed a normal
	// repair. Overrides Mode/SkipBackup.
	Resync bool
}

// reindexOptionsFor maps the operator's choices onto the port options. Resync
// (the corrupt-file repair) forces the rewrite + a backup regardless of the
// other choices - it drops bytes, so a rollback copy is mandatory. Otherwise
// the standard rewrite keeps a rollback backup unless SkipBackup trades it for
// a forced deep-verify; the surgical in-place mode cannot produce a backup by
// design (the file is patched, never duplicated). "auto" tries the patch
// first and falls back to the rewrite when the layout defeats it (streamed
// clusters, no head-discoverable index slot) - the fallback keeps the
// rewrite's backup semantics.
func reindexOptionsFor(run ReindexRunOptions) mkv.ReindexOptions {
	if run.Resync {
		// The adapter's resync path forces the rewrite + backup itself; carry
		// the flag and a deep-verify (the tolerant walk warrants the proof).
		return mkv.ReindexOptions{Resync: true, KeepBackup: true, DeepVerify: true}
	}
	deepVerify := run.DeepVerify || run.SkipBackup
	switch run.Mode {
	case mkv.ModeInPlace:
		return mkv.ReindexOptions{Mode: mkv.ModeInPlace, DeepVerify: deepVerify}
	case "auto":
		return mkv.ReindexOptions{Mode: mkv.ModeAuto, KeepBackup: !run.SkipBackup, DeepVerify: deepVerify}
	default:
		return mkv.ReindexOptions{Mode: mkv.ModeRewrite, KeepBackup: !run.SkipBackup, DeepVerify: deepVerify}
	}
}

// SupportsInPlace mirrors the reindexer's capability for the scan/UI layer.
func (s *SeekIndexService) SupportsInPlace() bool { return s.reindexer.SupportsInPlace() }

// StartReindexLibrary kicks a background job that re-scans the library then
// rebuilds the seek index of every un-indexed file. See ReindexRunOptions for
// the strategy/backup semantics. A backup-keeping batch is preflighted as a
// whole: when a volume cannot hold the sum of its candidates (each successful
// rewrite persists a .mkvgobak copy), the job refuses upfront instead of
// failing its tail hours in.
func (s *SeekIndexService) StartReindexLibrary(_ context.Context, libraryID string, run ReindexRunOptions) error {
	opts := reindexOptionsFor(run)
	var preflight func([]SeekStatus) error
	// A rolling window bounds the accumulation to ~N files, so the whole-sum
	// preflight would be a false refusal there; the per-file guard inside the
	// rewrite keeps the authority on transient space.
	if opts.KeepBackup && run.BackupWindow <= 0 {
		preflight = s.backupSpacePreflight
		// Fail fast on the last cached scan when one exists: the operator gets
		// the refusal synchronously - and translated, the RPC has a locale -
		// instead of a queued job that dies at its first phase. The job's own
		// re-scan below keeps the authority when the cache is absent or stale.
		if scan, _, ok := s.CachedScan(libraryID); ok {
			var kept []SeekStatus
			for _, it := range scan.Items {
				if !it.HasIndex {
					kept = append(kept, it)
				}
			}
			if err := preflight(kept); err != nil {
				return err
			}
		}
	}
	// Backup paths of the most recent successful rewrites, oldest first. Job
	// state: the batch runs on a single goroutine, so no locking needed.
	var window []string
	var epilogue func(context.Context)
	if run.BackupWindow > 0 {
		// Drain what the sliding purge left: the job ends with ZERO backups on
		// the volume, no manual purge pass needed. Each repair was already
		// validated before its swap; the window only existed to keep a short
		// abort-and-rollback horizon while the job was alive.
		epilogue = func(bg context.Context) {
			for _, p := range window {
				s.purgeRolledBackup(bg, p)
			}
		}
	}
	return s.startBatch(JobReindex, libraryID, preflight,
		func(it SeekStatus) bool { return !it.HasIndex },
		func(bg context.Context, path string) (bool, error) {
			res, err := s.reindexer.ReindexInPlace(bg, path, opts)
			if err != nil {
				return false, err
			}
			if !res.Skipped {
				s.refreshFileState(bg, path)
				hasBak := res.BackupPath != ""
				s.updateCachedFile(path, func(st *SeekStatus) {
					st.HasIndex = true
					st.HasBackup = hasBak
					st.DamagedBytes = 0
					st.Truncated = false
					st.TrailingJunk = false
				})
				if hasBak && run.BackupWindow > 0 {
					window = append(window, path)
					if len(window) > run.BackupWindow {
						s.purgeRolledBackup(bg, window[0])
						window = window[1:]
					}
				}
			}
			return res.Skipped, nil
		}, epilogue)
}

// rollingBackups is the job-local sliding window of kept .mkvgobak paths
// (oldest first). Zero max means "keep every backup".
type rollingBackups struct {
	window []string
	max    int
}

// reindexStep reindexes one file and folds the result into the cached scan,
// advancing the rolling backup window when one is configured. Shared by the
// reindex-only and combined-repair batches so their per-file semantics stay
// identical.
func (s *SeekIndexService) reindexStep(bg context.Context, path string, opts mkv.ReindexOptions, rb *rollingBackups) (bool, error) {
	res, err := s.reindexer.ReindexInPlace(bg, path, opts)
	if err != nil {
		return false, err
	}
	if res.Skipped {
		return true, nil
	}
	s.refreshFileState(bg, path)
	hasBak := res.BackupPath != ""
	s.updateCachedFile(path, func(st *SeekStatus) {
		st.HasIndex = true
		st.HasBackup = hasBak
		st.DamagedBytes = 0
		st.Truncated = false
		st.TrailingJunk = false
	})
	if hasBak && rb != nil && rb.max > 0 {
		rb.window = append(rb.window, path)
		if len(rb.window) > rb.max {
			s.purgeRolledBackup(bg, rb.window[0])
			rb.window = rb.window[1:]
		}
	}
	return false, nil
}

// drainRollingBackups purges whatever the sliding window still holds at the
// end of a job (so it finishes with none). No-op when the window is unused.
func (s *SeekIndexService) drainRollingBackups(bg context.Context, rb *rollingBackups) {
	if rb == nil {
		return
	}
	for _, p := range rb.window {
		s.purgeRolledBackup(bg, p)
	}
}

// needsReindex reports whether it belongs in a reindex/repair batch: when its
// index is missing/unusable, and - only under Resync, the operator's
// explicit consent to trade bytes for playability - also when it has real
// mid-file corruption a healthy index doesn't excuse (PlanHLS refuses such a
// file outright, however good its Cues are). Truncated is excluded from
// BOTH: its damage runs to EOF, unrecoverable by any reindex or resync, index
// or no index - a real attempt on one (proven against a real truncated file,
// no index) fails a verify step and reports a spurious failure for a file
// the operator can only fix by re-acquiring the source, never by repairing.
func needsReindex(it SeekStatus, resync bool) bool {
	if it.Truncated {
		return false
	}
	if !it.HasIndex {
		return true
	}
	return resync && it.DamagedBytes > 0
}

// reindexCandidates is the subset of items a reindex/repair batch will act
// on, shared by the space preflight and the batch's own selection so the two
// never drift apart.
func reindexCandidates(items []SeekStatus, resync bool) []SeekStatus {
	var out []SeekStatus
	for _, it := range items {
		if needsReindex(it, resync) {
			out = append(out, it)
		}
	}
	return out
}

// StartRepairLibrary is the "repair all" entry point: it reindexes every
// un-indexed file and, when includeAudio is set and a remuxer is wired, also
// repairs the audio start-delay of the flagged files - all in one job. A file
// carrying BOTH defects is only reindexed this run: remuxing it too would
// rename its fresh .mkvgobak (the true original) out from under itself, so its
// audio is left for the next run, when the index defect is gone. Backup and
// preflight semantics match StartReindexLibrary.
func (s *SeekIndexService) StartRepairLibrary(_ context.Context, libraryID string, run ReindexRunOptions, includeAudio bool) error {
	opts := reindexOptionsFor(run)
	syncPreflight := opts.KeepBackup && run.BackupWindow <= 0
	cacheHit := false
	if syncPreflight {
		if scan, _, ok := s.CachedScan(libraryID); ok {
			cacheHit = true
			if err := s.backupSpacePreflight(reindexCandidates(scan.Items, opts.Resync)); err != nil {
				return err
			}
		}
	}
	if !s.tryStart(JobReindex, libraryID) {
		return ErrJobRunning
	}
	doAudio := includeAudio && s.audioRemux != nil
	go func() {
		defer s.recoverJob()
		bg := s.jobContext()
		scan, _, ok := s.CachedScan(libraryID)
		if !ok {
			var err error
			if scan, err = s.scanAndCache(bg, libraryID); err != nil {
				s.finish(err)
				return
			}
			if syncPreflight && !cacheHit {
				if err := s.backupSpacePreflight(reindexCandidates(scan.Items, opts.Resync)); err != nil {
					s.finish(err)
					return
				}
			}
		}
		var idx, aud []string
		reindexing := map[string]bool{}
		for _, it := range scan.Items {
			if needsReindex(it, opts.Resync) {
				idx = append(idx, it.Path)
				reindexing[it.Path] = true
			}
		}
		if doAudio {
			for _, it := range scan.Items {
				if it.AudioDelaySec > AudioDelayFlagSec && !reindexing[it.Path] {
					aud = append(aud, it.Path)
				}
			}
		}
		s.setPhase(PhaseProcessing, len(idx)+len(aud))
		// One rolling window shared across both phases: index and audio
		// backups roll and drain together, so declining backups leaves NONE
		// on the volume (the audio backups used to survive the drain).
		rb := &rollingBackups{max: run.BackupWindow}
		s.processFiles(bg, JobReindex, idx, func(bg context.Context, path string) (bool, error) {
			return s.reindexStep(bg, path, opts, rb)
		})
		if len(aud) > 0 {
			s.processFiles(bg, JobRemuxAudio, aud, func(bg context.Context, path string) (bool, error) {
				return s.remuxAudioStep(bg, path, rb)
			})
		}
		s.drainRollingBackups(bg, rb)
		s.finish(nil)
	}()
	return nil
}

// StartReindexMedia rebuilds one media file's seek index in the background.
// The media lookup happens synchronously so unknown IDs fail fast. See
// StartReindexLibrary for the mode semantics.
func (s *SeekIndexService) StartReindexMedia(ctx context.Context, mediaID string, run ReindexRunOptions) error {
	opts := reindexOptionsFor(run)
	return s.startSingle(ctx, JobReindex, mediaID, func(bg context.Context, path string) (bool, error) {
		res, err := s.reindexer.ReindexInPlace(bg, path, opts)
		if err != nil {
			return false, err
		}
		if !res.Skipped {
			s.refreshFileState(bg, path)
			hasBak := res.BackupPath != ""
			s.updateCachedFile(path, func(st *SeekStatus) {
				st.HasIndex = true
				st.HasBackup = hasBak
				st.DamagedBytes = 0
				st.Truncated = false
				st.TrailingJunk = false
			})
			s.logDamagedRanges(path, res.DamagedRanges)
		}
		return res.Skipped, nil
	})
}

// logDamagedRanges surfaces what a Resync repair dropped so the operator sees
// exactly what was lost (bytes + approximate presentation time).
func (s *SeekIndexService) logDamagedRanges(path string, ranges []mkv.DamagedRange) {
	for _, r := range ranges {
		s.logger.Warn("seek-index: resync skipped a corrupted region (data lost)",
			slog.String("path", path),
			slog.Int64("bytes", r.EndOffset-r.StartOffset),
			slog.Int64("approx_start_ms", r.ApproxStartMs),
			slog.Int64("approx_end_ms", r.ApproxEndMs))
	}
}
