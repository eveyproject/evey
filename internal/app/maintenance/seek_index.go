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

// SeekStatus is the per-file diagnosis surfaced by a scan.
type SeekStatus struct {
	MediaID   string
	Title     string
	Path      string
	Container string
	HasIndex  bool // false → muxed without Cues → slow seek/resume
	HasBackup bool // a .mkvgobak exists → a prior reindex is rollback-able
	SizeBytes int64
}

// ScanResult bundles the per-file statuses with rollup counts.
type ScanResult struct {
	Items        []SeekStatus
	Total        int
	MissingIndex int
	WithBackup   int
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
	reindexer mkv.Reindexer
	catalog   MediaCatalog
	logger    *slog.Logger

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

var ebmlExts = map[string]bool{".mkv": true, ".mka": true, ".mks": true, ".mk3d": true, ".webm": true}

func isEBMLPath(p string) bool { return ebmlExts[strings.ToLower(filepath.Ext(p))] }

func containerOf(p string) string { return strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".") }

// IsReadOnlyFS reports whether err ultimately comes from a read-only mount —
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

// CachedScan returns the last completed scan for the scope, if any. Instant —
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
		// Fresh context: the job outlives the triggering request — the same
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

// StartReindexLibrary kicks a background job that re-scans the library then
// rebuilds the seek index of every un-indexed file (keeping backups).
func (s *SeekIndexService) StartReindexLibrary(_ context.Context, libraryID string) error {
	return s.startBatch(JobReindex, libraryID,
		func(it SeekStatus) bool { return !it.HasIndex },
		func(bg context.Context, path string) (bool, error) {
			res, err := s.reindexer.ReindexInPlace(bg, path, mkv.ReindexOptions{KeepBackup: true})
			if err != nil {
				return false, err
			}
			if !res.Skipped {
				s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = true; st.HasBackup = true })
			}
			return res.Skipped, nil
		})
}

// StartRollbackLibrary kicks a background job that re-scans the library then
// restores every backed-up file from its .mkvgobak.
func (s *SeekIndexService) StartRollbackLibrary(_ context.Context, libraryID string) error {
	return s.startBatch(JobRollback, libraryID,
		func(it SeekStatus) bool { return it.HasBackup },
		func(bg context.Context, path string) (bool, error) {
			if err := s.reindexer.RestoreBackup(bg, path); err != nil {
				return false, err
			}
			// A backup is only ever created when repairing an un-indexed file,
			// so the restored original is back to "no index".
			s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = false; st.HasBackup = false })
			return false, nil
		})
}

// StartReindexMedia rebuilds one media file's seek index in the background.
// The media lookup happens synchronously so unknown IDs fail fast.
func (s *SeekIndexService) StartReindexMedia(ctx context.Context, mediaID string) error {
	return s.startSingle(ctx, JobReindex, mediaID, func(bg context.Context, path string) (bool, error) {
		res, err := s.reindexer.ReindexInPlace(bg, path, mkv.ReindexOptions{KeepBackup: true})
		if err != nil {
			return false, err
		}
		if !res.Skipped {
			s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = true; st.HasBackup = true })
		}
		return res.Skipped, nil
	})
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

func (s *SeekIndexService) startBatch(kind, libraryID string, keep func(SeekStatus) bool, fn func(context.Context, string) (bool, error)) error {
	if !s.tryStart(kind, libraryID) {
		return ErrJobRunning
	}
	go func() {
		defer s.recoverJob()
		bg := context.Background()
		scan, err := s.scanAndCache(bg, libraryID)
		if err != nil {
			s.finish(err)
			return
		}
		var todo []string
		for _, it := range scan.Items {
			if keep(it) {
				todo = append(todo, it.Path)
			}
		}
		s.setPhase(PhaseProcessing, len(todo))
		s.processFiles(bg, kind, todo, fn)
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
	}

	s.mu.Lock()
	s.scans[jobKey(libraryID)] = cachedScan{res: res, at: time.Now()}
	s.mu.Unlock()
	return res, nil
}

func (s *SeekIndexService) probeFile(ctx context.Context, f MediaRef) (SeekStatus, bool) {
	fi, statErr := os.Stat(f.Path)
	if statErr != nil {
		// File recorded in the DB but missing/unreadable on disk — skip it
		// rather than reporting a misleading "no index".
		s.logger.Warn("seek-index scan: file unreadable, skipping",
			slog.String("path", f.Path), slog.Any("err", statErr))
		return SeekStatus{}, false
	}
	hasIdx, err := s.reindexer.HasSeekIndex(ctx, f.Path)
	if err != nil {
		s.logger.Warn("seek-index scan: parse failed, skipping",
			slog.String("path", f.Path), slog.Any("err", err))
		return SeekStatus{}, false
	}
	hasBak, bakErr := s.reindexer.HasBackup(ctx, f.Path)
	if bakErr != nil {
		// Backup status is secondary diagnostics — a probe failure must not
		// drop the file from the scan, just report it as "no backup".
		s.logger.Warn("seek-index scan: backup probe failed",
			slog.String("path", f.Path), slog.Any("err", bakErr))
	}
	return SeekStatus{
		MediaID:   f.MediaID,
		Title:     f.Title,
		Path:      f.Path,
		Container: containerOf(f.Path),
		HasIndex:  hasIdx,
		HasBackup: hasBak,
		SizeBytes: fi.Size(),
	}, true
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
		c.res.MissingIndex, c.res.WithBackup = 0, 0
		for _, it := range c.res.Items {
			if !it.HasIndex {
				c.res.MissingIndex++
			}
			if it.HasBackup {
				c.res.WithBackup++
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
	return true
}

func (s *SeekIndexService) setPhase(phase string, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.Phase = phase
	s.cur.Total = total
	s.cur.Done, s.cur.Failed, s.cur.Skipped = 0, 0, 0
	s.cur.CurrentPath = ""
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
