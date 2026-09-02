package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// seek_index_reindex.go holds the Cues-rebuild + "repair all" paths: the
// operator's ReindexRunOptions, the per-file reindex step and the rolling
// backup window shared by reindex and combined repair.

// ErrBackupRequired refuses a run that would alter a file's CONTENT with no
// way back. Deep-verify proves the result is well formed; it never proves the
// operator wanted what the file now contains, and a resync has just thrown
// bytes away. Kept distinct so the API layer can say why in the operator's
// language instead of surfacing a generic failure.
var ErrBackupRequired = errors.New("this repair alters the file's content and cannot run without a backup")

// altersContent reports whether a run rewrites what the file PLAYS, as opposed
// to how it is indexed. A reindex rebuilds a container's index and leaves the
// picture and sound byte for byte; a resync discards the damaged span. The
// distinction decides two things: whether the run may trade its backup away,
// and whether the rolling window may purge it.
//
// The 2026-07-13 batch is why the second one exists. It kept a backup of all
// 98 files it rewrote and the window then purged every one of them, so when the
// damage surfaced seven weeks later there was nothing to restore.
func (o ReindexRunOptions) altersContent() bool { return o.Resync || o.ResyncDamaged }

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
	// ResyncDamaged, for a whole-library repair, also rewrites the files
	// whose index is fine but which carry mid-file corruption - in resync
	// mode (skip the damaged stretch, lose those bytes, mandatory backup and
	// deep verify), which is the ONLY way such a file becomes playable. It
	// is the operator's explicit consent to that loss, asked in the confirm
	// dialog with the count and the bytes at stake; unlike Resync it leaves
	// every other candidate on the normal, lossless path.
	ResyncDamaged bool
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
	if run.altersContent() && run.SkipBackup {
		return ErrBackupRequired
	}
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
			if err := preflight(reindexCandidates(scan.Items, opts.Resync)); err != nil {
				return err
			}
		}
	}
	// Backup paths of the most recent successful rewrites, oldest first. Job
	// state: the batch runs on a single goroutine, so no locking needed.
	// The per-file work goes through the SAME reindexStep as the combined
	// repair - this path used to carry its own copy of it, and a guard added
	// to one was silently absent from the other.
	rb := &rollingBackups{max: run.BackupWindow, keepAll: run.altersContent()}
	var epilogue func(context.Context)
	if run.BackupWindow > 0 && !run.altersContent() {
		// Drain what the sliding purge left: the job ends with ZERO backups on
		// the volume, no manual purge pass needed. Each repair was already
		// validated before its swap; the window only existed to keep a short
		// abort-and-rollback horizon while the job was alive.
		epilogue = func(bg context.Context) { s.drainRollingBackups(bg, rb) }
	}
	return s.startBatch(JobReindex, libraryID, preflight,
		func(it SeekStatus) bool { return needsReindex(it, opts.Resync) },
		func(bg context.Context, path string) (bool, error) {
			return s.reindexStep(bg, path, opts, rb)
		}, epilogue)
}

// rollingBackups is the job-local sliding window of kept .mkvgobak paths
// (oldest first). Zero max means "keep every backup".
type rollingBackups struct {
	window []string
	max    int
	// keepAll suspends the purge for a repair that alters the file's content:
	// its backup is the only way back, and bounding disk use is not worth an
	// unrecoverable original.
	keepAll bool
}

// reindexStep reindexes one file and folds the result into the cached scan,
// advancing the rolling backup window when one is configured. Shared by the
// reindex-only and combined-repair batches so their per-file semantics stay
// identical.
func (s *SeekIndexService) reindexStep(bg context.Context, path string, opts mkv.ReindexOptions, rb *rollingBackups) (bool, error) {
	// The entry is opened BEFORE the rewrite and closed whatever happens, so a
	// file can never be rewritten without a trace - the exact gap that made the
	// 2026-07-13 batch unexplainable seven weeks later.
	rec := s.openRepairRecord(bg, path, opts)

	// What the file sounded like BEFORE, so a loss can be proven rather than
	// suspected. Measured first because after the rewrite the original is
	// only reachable through its backup.
	audibleBefore, haveBaseline := s.audibleTracks(bg, path)
	rec.AudibleBefore = audibleBefore

	res, err := s.reindexer.ReindexInPlace(bg, path, opts)
	if err != nil {
		rec.Outcome, rec.Error = "failed", err.Error()
		s.journalRepair(bg, rec)
		return false, err
	}
	if res.Skipped {
		rec.Outcome = "skipped"
		s.journalRepair(bg, rec)
		return true, nil
	}
	rec.BackupPath = res.BackupPath
	if haveBaseline {
		after, ok := s.audibleTracks(bg, path)
		rec.AudibleAfter = after
		if err := s.guardAudioSurvived(bg, path, audibleBefore, after, ok, res.BackupPath != ""); err != nil {
			rec.Outcome, rec.Error = "rolled_back", err.Error()
			rec.BackupPath = "" // the backup was consumed putting the original back
			s.journalRepair(bg, rec)
			return false, err
		}
	}
	s.refreshFileState(bg, path)
	hasBak := res.BackupPath != ""
	s.noteReindexed(bg, path, hasBak)
	if hasBak && rb != nil && rb.max > 0 && !rb.keepAll {
		rb.window = append(rb.window, path)
		if len(rb.window) > rb.max {
			s.purgeRolledBackup(bg, rb.window[0])
			rb.window = rb.window[1:]
		}
	}
	rec.Outcome = "applied"
	rec.SizeAfter = fileSizeOf(path)
	s.journalRepair(bg, rec)
	return false, nil
}

// openRepairRecord captures what the file was before anything touched it, and
// the evidence that justified touching it. Evidence is the field that tells a
// genuine repair from a rewrite performed on a bare declaration: a coherence
// measurement is proof, a declared offset on its own is not - and acting on the
// latter alone is precisely what broke 98 files.
func (s *SeekIndexService) openRepairRecord(ctx context.Context, path string, opts mkv.ReindexOptions) RepairRecord {
	rec := RepairRecord{
		Path:       path,
		Op:         RepairReindex,
		Started:    time.Now(),
		SizeBefore: fileSizeOf(path),
		Evidence:   RepairEvidence{Method: "declared-defect"},
	}
	if opts.Resync {
		rec.Op = RepairResync
	}
	if st, ok := s.statusOf(path); ok {
		rec.MediaID = st.MediaID
		if st.AudioDelaySec > 0 {
			rec.DelaysBefore = []TrackDelay{{DelaySec: st.AudioDelaySec}}
		}
		if st.AudioTracksChecked {
			// A measurement ran: say what it found, including a lag too weak to
			// act on. Silence here would read as "never looked".
			rec.Evidence = RepairEvidence{
				Method:     "track-coherence",
				LagSec:     st.AudioTracksLagSec,
				Confidence: AudioCoherenceMinConfidence,
				Detail:     st.IndexSparseDetail,
			}
		} else if st.IndexSparseDetail != "" {
			rec.Evidence.Detail = st.IndexSparseDetail
		}
	}
	rec.Job = s.runningJobKind()
	_ = ctx
	return rec
}

// runningJobKind names the batch this file belongs to, so a whole night's work
// can be reviewed as one thing instead of reconstructed from file timestamps.
func (s *SeekIndexService) runningJobKind() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.Kind
}

// statusOf returns the cached diagnosis of a path, from whichever scope holds
// it. Used to fill a journal entry with what was known at decision time.
func (s *SeekIndexService) statusOf(path string) (SeekStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.scans {
		for _, it := range c.res.Items {
			if it.Path == path {
				return it, true
			}
		}
	}
	return SeekStatus{}, false
}

// fileSizeOf is best effort: a size we could not read must not cost a repair.
func fileSizeOf(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// audibleTracks measures how many audio tracks carry sound, and says whether
// the measurement is usable at all. A prober that is not wired, or one that
// fails, yields no baseline: the repair then runs as it always did rather
// than being blocked by a broken ffmpeg.
func (s *SeekIndexService) audibleTracks(ctx context.Context, path string) (int, bool) {
	if s.audible == nil {
		return 0, false
	}
	n, err := s.audible.ProbeAudibleAudioTracks(ctx, path)
	if err != nil {
		s.logger.Warn("seek-index: audio presence unmeasurable, repairing unguarded",
			slog.String("path", path), slog.Any("err", err))
		return 0, false
	}
	return n, true
}

// guardAudioSurvived re-measures a just-rewritten file and, when the rewrite
// cost it an audible track, puts the backup back and reports the file as
// failed. Anything short of a PROVEN loss leaves the repair standing: a file
// that had no sound to lose is fine, and a second measurement that could not
// run is not evidence.
func (s *SeekIndexService) guardAudioSurvived(ctx context.Context, path string, before, after int, measured, hasBackup bool) error {
	if before == 0 {
		return nil // nothing to lose, nothing to prove
	}
	if !measured || after >= before {
		return nil
	}
	s.logger.Error("seek-index: the repair silenced audio, rolling it back",
		slog.String("path", path), slog.Int("audible_before", before), slog.Int("audible_after", after))
	if !hasBackup {
		// No backup to go back to: say so plainly instead of pretending the
		// file was restored. This is why a rewrite keeps one.
		return fmt.Errorf("%w (%d audible before, %d after) and no backup exists to restore",
			ErrAudioLost, before, after)
	}
	if rerr := s.reindexer.RestoreBackup(ctx, path); rerr != nil {
		return fmt.Errorf("%w (%d audible before, %d after); restoring the backup ALSO failed: %w",
			ErrAudioLost, before, after, rerr)
	}
	// The original is back: it carries its pre-repair defect again and its
	// backup is gone, exactly like an operator-driven rollback.
	s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = false; st.HasBackup = false })
	return fmt.Errorf("%w (%d audible before, %d after) - the original has been restored",
		ErrAudioLost, before, after)
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
	// Already rebuilt once and still too coarse: the encode's keyframes, not
	// the index, are the limit. Re-offering it every run is the sterile loop.
	if it.SeekCoarse() || it.PictureMissing {
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

// StartRepairLibrary is the "repair all" entry point: ONE job that acts on
// every file whose verdict is repairable, so the count the button shows is the
// count the job processes. In order: index rebuilds (plus, with ResyncDamaged,
// the resync of damaged-but-indexed files), then the audio start-delay remux
// of the flagged files, then the container rename of .mkv files that are
// really MP4. A file carrying an index defect AND an audio one is only
// reindexed this run: remuxing it too would rename its fresh .mkvgobak (the
// true original) out from under itself, so its audio is left for the next
// run, when the index defect is gone. Backup and preflight semantics match
// StartReindexLibrary; the damaged files' resync always keeps a backup.
func (s *SeekIndexService) StartRepairLibrary(_ context.Context, libraryID string, run ReindexRunOptions) error {
	opts := reindexOptionsFor(run)
	syncPreflight := opts.KeepBackup && run.BackupWindow <= 0
	cacheHit := false
	if syncPreflight {
		if scan, _, ok := s.CachedScan(libraryID); ok {
			cacheHit = true
			if err := s.backupSpacePreflight(repairAllCandidates(scan.Items, opts.Resync, run.ResyncDamaged)); err != nil {
				return err
			}
		}
	}
	if !s.tryStart(JobReindex, libraryID) {
		return ErrJobRunning
	}
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
				if err := s.backupSpacePreflight(repairAllCandidates(scan.Items, opts.Resync, run.ResyncDamaged)); err != nil {
					s.finish(err)
					return
				}
			}
		}
		var idx, resync, containers []string
		touched := map[string]bool{}
		for _, it := range scan.Items {
			switch {
			case needsReindex(it, opts.Resync):
				idx = append(idx, it.Path)
				touched[it.Path] = true
			case run.ResyncDamaged && needsResyncForDamage(it):
				resync = append(resync, it.Path)
				touched[it.Path] = true
			}
		}
		// NOTHING is collected here any more, and that is the point. "Repair
		// all" used to retime every file whose audio started more than
		// AudioDelayFlagSec after the picture. That threshold has no upper
		// bound, so a DELIBERATE alignment was treated like a 150 ms mux slip:
		// on a prod title (prod 2026-07-13) it erased a 12.000 s dub offset,
		// the French track then played 12 s early in every player, and no
		// backup was kept. A start offset is not evidence of a defect - the
		// container is usually right and every player honours it. The audio
		// remux stays available as a deliberate, per-file action; a bulk job
		// that rewrites media on a threshold does not.
		if s.relocator != nil {
			for _, it := range scan.Items {
				if it.WrongContainer {
					containers = append(containers, it.Path)
				}
			}
		}
		s.setPhase(PhaseProcessing, len(idx)+len(resync)+len(containers))
		// One rolling window shared across the phases: index and audio
		// backups roll and drain together, so declining backups leaves NONE
		// on the volume (the audio backups used to survive the drain). The
		// resync backups are NOT in it: a repair that loses bytes keeps its
		// original whatever the operator chose for the lossless ones.
		rb := &rollingBackups{max: run.BackupWindow, keepAll: run.altersContent()}
		s.processFiles(bg, JobReindex, idx, func(bg context.Context, path string) (bool, error) {
			return s.reindexStep(bg, path, opts, rb)
		})
		if len(resync) > 0 {
			resyncOpts := reindexOptionsFor(ReindexRunOptions{Resync: true})
			s.processFiles(bg, JobReindex, resync, func(bg context.Context, path string) (bool, error) {
				return s.reindexStep(bg, path, resyncOpts, nil)
			})
		}
		if len(containers) > 0 {
			s.processFiles(bg, JobFixContainer, containers, s.fixContainerStep)
		}
		s.drainRollingBackups(bg, rb)
		s.finish(nil)
	}()
	return nil
}

// needsResyncForDamage picks the files ONLY a resync can make playable: index
// fine, mid-file corruption, not an incomplete download (whose damage runs to
// the end - nothing to resynchronise onto).
func needsResyncForDamage(it SeekStatus) bool {
	return it.HasIndex && it.DamagedBytes > 0 && !it.Truncated && !it.SeekCoarse()
}

// repairAllCandidates is every file the "repair all" job will REWRITE - the
// set the backup space preflight must size.
func repairAllCandidates(items []SeekStatus, resync, resyncDamaged bool) []SeekStatus {
	out := reindexCandidates(items, resync)
	if resyncDamaged {
		for _, it := range items {
			if !needsReindex(it, resync) && needsResyncForDamage(it) {
				out = append(out, it)
			}
		}
	}
	return out
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
			s.noteReindexed(bg, path, res.BackupPath != "")
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
