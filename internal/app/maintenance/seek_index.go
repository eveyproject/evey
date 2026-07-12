// Package maintenance holds admin-triggered media maintenance use cases that
// operate on the on-disk library rather than the database. The seek-index tools
// detect and repair MKV/WebM files muxed without a Cues index (the cause of slow
// seek/resume) using the fail-safe mkv.Reindexer port.
//
// Every entry point is asynchronous: scanning walks thousands of files on a
// (possibly network-mounted) library and repairing rewrites whole multi-GB
// files, so a synchronous RPC would time out behind any reverse proxy (the
// prod 504). Callers start a job and poll Status(); completed scans are cached
// in memory and served instantly.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// ErrJobRunning is returned by the Start* entry points when a seek-index job
// is already in flight. Handlers map it to a user-actionable "try again later"
// instead of a 500. Jobs are serialized globally to bound disk I/O on weak
// hosts (NAS), matching the sequential batch design.
var ErrJobRunning = errors.New("a seek-index job is already running")

// Job kinds and phases surfaced by Status().
const (
	JobScan     = "scan"
	JobReindex  = "reindex"
	JobRollback = "rollback"
	JobPurge    = "purge" // delete every .mkvgobak backup in scope
	// JobRemuxAudio rewrites files whose audio content starts late (repack
	// mux defect) so the file itself is fixed, not just the playback.
	JobRemuxAudio = "remux_audio"

	PhaseScanning   = "scanning"
	PhaseProcessing = "processing"
)

// maxFailures bounds the per-job failure list kept for the status endpoint.
const maxFailures = 50

// MediaRef is the minimal media-file info the seek-index tools need. It is
// resolved from the library by a MediaCatalog adapter.
type MediaRef struct {
	MediaID string
	Title   string
	Path    string
}

// MediaCatalog lists and looks up media files. Satisfied by an adapter over the
// library query service (kept narrow so this package doesn't depend on the
// whole library app service).
type MediaCatalog interface {
	ListMediaFiles(ctx context.Context, libraryID string) ([]MediaRef, error)
	GetMediaFile(ctx context.Context, mediaID string) (MediaRef, error)
}

// FileStateRefresher re-stamps the change-detection fingerprint (size + mtime)
// the library scan keys on for a media file. After a reindex rewrites a
// container in place, the bytes change but the logical content - streams,
// keyframe times, duration - does not; refreshing the stored fingerprint makes
// the next scan treat the file as UNCHANGED instead of re-probing it and
// dropping the keyframe index, which would force a transcode window (the copy
// rung needs keyframes_ms) until the keyframe backfill worker catches up.
// Implemented by the media-item repository; optional (nil disables the refresh,
// falling back to the re-probe path).
type FileStateRefresher interface {
	RefreshFileState(ctx context.Context, path string, size int64, modTime time.Time) error
}

// AudioDelayProber measures how far a file's audio content starts AFTER its
// video (max across audio tracks, seconds) - the repack mux defect that plays
// as a constant A/V desync. Satisfied by the ffmpeg prober; optional (nil
// skips the diagnosis). Kept separate from the Cues probe: it is a different
// defect class, repaired by a remux, not a reindex.
type AudioDelayProber interface {
	ProbeMaxAudioStartDelaySec(ctx context.Context, path string) (float64, error)
}

// AudioDelayFlagSec is the floor above which a probed audio start delay is
// surfaced as a defect. Matches the HLS builders' cancel floor: legitimate
// codec priming sits at ~20-40ms, the defect class at hundreds of ms.
const AudioDelayFlagSec = 0.1

// AudioDelayRemuxer repairs the audio start-delay defect AT THE FILE level:
// a remux that shifts the late audio content back onto the video, with the
// same fail-safe contract as the reindexer (rewrite to a temp, re-verify,
// atomic swap, original kept as .mkvgobak; any failure leaves the original
// byte-for-byte intact). skipped=true means no track sits above the defect
// floor anymore (nothing to fix); backupPath is the kept original ("" when
// skipped). Satisfied by the ffmpeg adapter; optional (nil disables the
// remux jobs). Primitive-typed on purpose - the driven adapter implements it
// without importing this package.
type AudioDelayRemuxer interface {
	RemuxAudioDelay(ctx context.Context, path string) (backupPath string, skipped bool, err error)
}

// SeekStatus is the per-file diagnosis surfaced by a scan.
type SeekStatus struct {
	MediaID   string
	Title     string
	Path      string
	Container string
	HasIndex  bool // false → muxed without Cues → slow seek/resume
	HasBackup bool // a .mkvgobak exists → a prior reindex is rollback-able
	SizeBytes int64
	// AudioDelaySec > AudioDelayFlagSec → the audio track starts that far
	// after the video (mux defect → constant A/V desync on copied audio).
	// Playback with TRANSCODED audio auto-cancels it; the flag tells the
	// operator the file itself carries the defect. 0 when not probed.
	AudioDelaySec float64
	// Truncated → the file is an incomplete download whose damage runs to the
	// end: no tool can recover the missing tail, a resync would only produce a
	// playable prefix. The operator must re-acquire the source. Only a
	// diagnosing scan (WithDiagnoser) can tell this apart from Damaged.
	Truncated bool
	// DamagedBytes > 0 → the file has mid-file corruption a resync repair can
	// skip past, losing this many bytes (repairable in full otherwise). 0 when
	// clean or not diagnosed.
	DamagedBytes int64
}

// ScanResult bundles the per-file statuses with rollup counts.
type ScanResult struct {
	Items        []SeekStatus
	Total        int
	MissingIndex int
	WithBackup   int
	AudioDelayed int // count with AudioDelaySec > AudioDelayFlagSec
	Truncated    int // count that are incomplete downloads (re-acquire, not repair)
	Damaged      int // count with mid-file corruption (resync-repairable)
}

// Failure records one file a batch job could not process.
type Failure struct {
	Path     string
	Error    string
	ReadOnly bool // the failure came from a read-only mount (:ro volume)
}

// JobSummary describes the last finished job.
type JobSummary struct {
	Kind       string
	Scope      string // "" = all libraries, else the library ID
	Done       int
	Failed     int
	Skipped    int
	Total      int
	FinishedAt time.Time
	Err        string // job-level error (e.g. the scan itself failed)
	ReadOnly   bool   // at least one failure came from a read-only mount
	Failures   []Failure
}

// JobStatus is the pollable state of the seek-index job runner.
type JobStatus struct {
	Running     bool
	Kind        string
	Scope       string
	Phase       string
	Done        int
	Failed      int
	Skipped     int
	Total       int
	CurrentPath string
	StartedAt   time.Time
	Last        *JobSummary
}

type cachedScan struct {
	res ScanResult
	at  time.Time
}

// SeekIndexService is the admin-facing seek-index maintenance use case.
type SeekIndexService struct {
	reindexer  mkv.Reindexer
	catalog    MediaCatalog
	fileState  FileStateRefresher // optional; see WithFileStateRefresher
	audioDelay AudioDelayProber   // optional; see WithAudioDelayProber
	audioRemux AudioDelayRemuxer  // optional; see WithAudioRemuxer
	diagnoser  mkv.Diagnoser      // optional; see WithDiagnoser
	logger     *slog.Logger

	stateDir string // "" = no persistence (WithStateDir enables it)

	mu          sync.Mutex
	running     bool
	cur         JobStatus // valid while running (Last excluded)
	curFailures jobFailures
	last        *JobSummary
	scans       map[string]cachedScan // scope key ("*" or libraryID) → last completed scan
}

func NewSeekIndexService(reindexer mkv.Reindexer, catalog MediaCatalog, logger *slog.Logger) *SeekIndexService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SeekIndexService{
		reindexer: reindexer,
		catalog:   catalog,
		logger:    logger.With("component", "seek_index"),
		scans:     map[string]cachedScan{},
	}
}

// WithFileStateRefresher wires the post-reindex fingerprint refresh so a
// rewritten file is not re-probed (and its keyframe index dropped) by the next
// library scan. Optional and chainable; without it the reindex still succeeds,
// the file is just re-probed later (a transcode window until keyframes
// backfill).
func (s *SeekIndexService) WithFileStateRefresher(r FileStateRefresher) *SeekIndexService {
	s.fileState = r
	return s
}

// WithAudioDelayProber wires the audio start-delay diagnosis into scans (one
// header-only ffprobe per file - the scan stays an async job with progress).
// Optional and chainable; without it AudioDelaySec stays 0 everywhere.
func (s *SeekIndexService) WithAudioDelayProber(p AudioDelayProber) *SeekIndexService {
	s.audioDelay = p
	return s
}

// WithAudioRemuxer wires the audio start-delay REPAIR (a validated remux).
// Optional and chainable; without it the remux entry points fail cleanly.
func (s *SeekIndexService) WithAudioRemuxer(r AudioDelayRemuxer) *SeekIndexService {
	s.audioRemux = r
	return s
}

// WithDiagnoser wires mkvgo's one-call triage (ops.Diagnose) into scans. When
// present it SUBSUMES the stacked HasSeekIndex + audio-delay probes with a
// single head-mostly read that additionally: catches a present-but-unusable
// index (audio-keyed or stale cues) as a defect, and classifies corruption as
// truncated (re-acquire the source) vs damaged (resync-repairable, N bytes
// lost) - the signal that routes the repair. Optional and chainable; without
// it scans fall back to the separate HasSeekIndex + AudioDelayProber probes.
func (s *SeekIndexService) WithDiagnoser(d mkv.Diagnoser) *SeekIndexService {
	s.diagnoser = d
	return s
}

// ErrRemuxUnavailable is returned by the remux entry points when no remuxer
// is wired (e.g. a build without ffmpeg).
var ErrRemuxUnavailable = errors.New("audio remux is not available in this build")

// remuxAudioFile is the shared per-file step of both remux entry points.
func (s *SeekIndexService) remuxAudioFile(bg context.Context, path string) (bool, error) {
	return s.remuxAudioStep(bg, path, nil)
}

// remuxAudioStep repairs one file's audio delay and folds the result into the
// cached scan, advancing the rolling backup window when one is configured -
// so a combined "repair all" that is NOT keeping backups drains the audio
// backups too (they used to persist regardless of the operator's choice).
func (s *SeekIndexService) remuxAudioStep(bg context.Context, path string, rb *rollingBackups) (bool, error) {
	bak, skipped, err := s.audioRemux.RemuxAudioDelay(bg, path)
	if err != nil {
		return false, err
	}
	if skipped {
		return true, nil
	}
	// The video stream and its keyframe TIMES are copied untouched; refresh
	// the fingerprint so the next scan keeps the stream records instead of
	// re-probing (same rationale as the reindex path).
	s.refreshFileState(bg, path)
	hasBak := bak != ""
	s.updateCachedFile(path, func(st *SeekStatus) { st.AudioDelaySec = 0; st.HasBackup = hasBak })
	if hasBak && rb != nil && rb.max > 0 {
		rb.window = append(rb.window, path)
		if len(rb.window) > rb.max {
			s.purgeRolledBackup(bg, rb.window[0])
			rb.window = rb.window[1:]
		}
	}
	return false, nil
}

// StartRemuxAudioLibrary kicks a background job that repairs every file the
// scan flagged with an audio start delay above the defect floor.
func (s *SeekIndexService) StartRemuxAudioLibrary(_ context.Context, libraryID string) error {
	if s.audioRemux == nil {
		return ErrRemuxUnavailable
	}
	return s.startBatch(JobRemuxAudio, libraryID, nil,
		func(it SeekStatus) bool { return it.AudioDelaySec > AudioDelayFlagSec },
		s.remuxAudioFile, nil)
}

// StartRemuxAudioMedia repairs one media file's audio start delay in the
// background. The media lookup happens synchronously so unknown IDs fail fast.
func (s *SeekIndexService) StartRemuxAudioMedia(ctx context.Context, mediaID string) error {
	if s.audioRemux == nil {
		return ErrRemuxUnavailable
	}
	return s.startSingle(ctx, JobRemuxAudio, mediaID, s.remuxAudioFile)
}

// refreshFileState re-stamps the scan fingerprint (size+mtime) on a file a
// reindex just rewrote, so the next library scan sees it UNCHANGED and keeps
// the existing streams + keyframe index instead of re-probing (which nulls
// keyframes_ms and forces a transcode window). Best-effort: any failure only
// means the file is re-probed later - the pre-refresh behaviour - so it is
// logged, never fatal to the job. Runs right after the swap to beat the ~2 min
// periodic scan to the file.
func (s *SeekIndexService) refreshFileState(ctx context.Context, path string) {
	if s.fileState == nil {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		s.logger.Warn("seek-index: stat after reindex failed (file will be re-probed)",
			slog.String("path", path), slog.Any("err", err))
		return
	}
	if err := s.fileState.RefreshFileState(ctx, path, fi.Size(), fi.ModTime()); err != nil {
		s.logger.Warn("seek-index: refresh file fingerprint failed (file will be re-probed)",
			slog.String("path", path), slog.Any("err", err))
	}
}

var ebmlExts = map[string]bool{".mkv": true, ".mka": true, ".mks": true, ".mk3d": true, ".webm": true}

func isEBMLPath(p string) bool { return ebmlExts[strings.ToLower(filepath.Ext(p))] }

func containerOf(p string) string { return strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".") }

// IsReadOnlyFS reports whether err ultimately comes from a read-only mount -
// the common hardening where media volumes are bound :ro in docker-compose.
// The string fallback covers error chains that didn't wrap with %w.
func IsReadOnlyFS(err error) bool {
	return errors.Is(err, syscall.EROFS) || strings.Contains(err.Error(), "read-only file system")
}

func jobKey(libraryID string) string {
	if libraryID == "" {
		return "*"
	}
	return libraryID
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
		// Fresh context: the job outlives the triggering request - the same
		// pattern as the enrichment / album-art kickers.
		bg := context.Background()
		_, err := s.scanAndCache(bg, libraryID)
		s.finish(err)
	}()
	return nil
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
				s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = true; st.HasBackup = hasBak })
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
	s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = true; st.HasBackup = hasBak })
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

// missingIndexItems is the un-indexed subset used by the space preflight.
func missingIndexItems(items []SeekStatus) []SeekStatus {
	var out []SeekStatus
	for _, it := range items {
		if !it.HasIndex {
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
			if err := s.backupSpacePreflight(missingIndexItems(scan.Items)); err != nil {
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
		bg := context.Background()
		scan, _, ok := s.CachedScan(libraryID)
		if !ok {
			var err error
			if scan, err = s.scanAndCache(bg, libraryID); err != nil {
				s.finish(err)
				return
			}
			if syncPreflight && !cacheHit {
				if err := s.backupSpacePreflight(missingIndexItems(scan.Items)); err != nil {
					s.finish(err)
					return
				}
			}
		}
		var idx, aud []string
		reindexing := map[string]bool{}
		for _, it := range scan.Items {
			if !it.HasIndex {
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

// purgeRolledBackup deletes one media file's backup as it leaves the rolling
// window (or at the end-of-job drain) and folds the deletion into the cached
// scan. Best-effort: a failure only leaves that one backup behind,
// reclaimable via the regular purge action.
func (s *SeekIndexService) purgeRolledBackup(ctx context.Context, path string) {
	if err := s.reindexer.DeleteBackup(ctx, path); err != nil {
		s.logger.Warn("seek-index: rolling backup purge failed (backup left in place)",
			slog.String("path", path), slog.Any("err", err))
		return
	}
	s.updateCachedFile(path, func(st *SeekStatus) { st.HasBackup = false })
}

// StartRollbackLibrary kicks a background job that re-scans the library then
// restores every backed-up file from its .mkvgobak.
func (s *SeekIndexService) StartRollbackLibrary(_ context.Context, libraryID string) error {
	return s.startBatch(JobRollback, libraryID, nil,
		func(it SeekStatus) bool { return it.HasBackup },
		func(bg context.Context, path string) (bool, error) {
			if err := s.reindexer.RestoreBackup(bg, path); err != nil {
				return false, err
			}
			// A backup is only ever created when repairing an un-indexed file,
			// so the restored original is back to "no index".
			s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = false; st.HasBackup = false })
			return false, nil
		}, nil)
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
			s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = true; st.HasBackup = hasBak })
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

// DeleteBackupMedia removes one media file's reindex backup, reclaiming the
// duplicated disk space. Synchronous - an unlink is instant, no job needed -
// and allowed while a job runs (it doesn't touch the media file itself).
func (s *SeekIndexService) DeleteBackupMedia(ctx context.Context, mediaID string) error {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("seek-index delete-backup: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("seek-index delete-backup: media %s has no file", mediaID)
	}
	if err := s.reindexer.DeleteBackup(ctx, f.Path); err != nil {
		return err
	}
	s.updateCachedFile(f.Path, func(st *SeekStatus) { st.HasBackup = false })
	return nil
}

// StartPurgeBackups kicks a background job that re-scans the scope then
// deletes every reindex backup in it - the bulk "the repairs are confirmed
// fine, give me my disk back" action.
func (s *SeekIndexService) StartPurgeBackups(_ context.Context, libraryID string) error {
	return s.startBatch(JobPurge, libraryID, nil,
		func(it SeekStatus) bool { return it.HasBackup },
		func(bg context.Context, path string) (bool, error) {
			if err := s.reindexer.DeleteBackup(bg, path); err != nil {
				return false, err
			}
			s.updateCachedFile(path, func(st *SeekStatus) { st.HasBackup = false })
			return false, nil
		}, nil)
}

// StartRollbackMedia restores one media file from its backup in the background.
func (s *SeekIndexService) StartRollbackMedia(ctx context.Context, mediaID string) error {
	return s.startSingle(ctx, JobRollback, mediaID, func(bg context.Context, path string) (bool, error) {
		if err := s.reindexer.RestoreBackup(bg, path); err != nil {
			return false, err
		}
		s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = false; st.HasBackup = false })
		return false, nil
	})
}

func (s *SeekIndexService) startSingle(ctx context.Context, kind, mediaID string, fn func(context.Context, string) (bool, error)) error {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("seek-index %s: lookup media %s: %w", kind, mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("seek-index %s: media %s has no file", kind, mediaID)
	}
	if !s.tryStart(kind, "") {
		return ErrJobRunning
	}
	s.setPhase(PhaseProcessing, 1)
	go func() {
		defer s.recoverJob()
		bg := context.Background()
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
	if !s.tryStart(kind, libraryID) {
		return ErrJobRunning
	}
	go func() {
		defer s.recoverJob()
		bg := context.Background()
		// Phase 1: reuse the cached scan when one exists - a full re-walk is
		// minutes of per-file probes and every repair re-checks its own file
		// anyway (an already-indexed file is Skipped, a vanished one fails
		// cleanly). The scan job itself always walks fresh.
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
		st.HasIndex = !d.HasIndexDefect()
		st.AudioDelaySec = float64(d.MaxAudioDelayMs()) / 1000
		st.Truncated = d.Truncated
		st.DamagedBytes = d.DamagedBytes
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
		c.res.Truncated, c.res.Damaged = 0, 0
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
		}
		s.scans[key] = c
	}
}

// ─── job runner state ────────────────────────────────────────────────────────

func (s *SeekIndexService) tryStart(kind, libraryID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	s.cur = JobStatus{
		Kind:      kind,
		Scope:     libraryID,
		Phase:     PhaseScanning,
		StartedAt: time.Now(),
	}
	s.saveJobMarker()
	return true
}

func (s *SeekIndexService) setPhase(phase string, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.Phase = phase
	s.cur.Total = total
	s.cur.Done, s.cur.Failed, s.cur.Skipped = 0, 0, 0
	s.cur.CurrentPath = ""
	s.saveJobMarker()
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
	s.saveJobMarker()
	s.mu.Unlock()
}

// jobFailures accumulates outside JobStatus so Status() copies stay cheap.
type jobFailures struct {
	list     []Failure
	readOnly bool
}

func (s *SeekIndexService) recordFailure(path string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.Done++
	s.cur.Failed++
	ro := IsReadOnlyFS(err)
	if ro {
		s.curFailures.readOnly = true
	}
	if len(s.curFailures.list) < maxFailures {
		s.curFailures.list = append(s.curFailures.list, Failure{Path: path, Error: err.Error(), ReadOnly: ro})
	}
	s.saveJobMarker()
}

func (s *SeekIndexService) finish(jobErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		slog.String("err", sum.Err))
}
