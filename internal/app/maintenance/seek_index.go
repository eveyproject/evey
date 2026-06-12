// Package maintenance holds admin-triggered media maintenance use cases that
// operate on the on-disk library rather than the database. The seek-index tools
// detect and repair MKV/WebM files muxed without a Cues index (the cause of slow
// seek/resume) using the fail-safe mkv.Reindexer port.
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

	"github.com/evey-project/evey/internal/domain/mkv"
)

// ErrJobRunning is returned by the batch entry points when a seek-index batch
// is already in flight for the requested scope (or a conflicting one). Handlers
// can map it to a user-actionable "try again later" instead of a 500.
var ErrJobRunning = errors.New("a seek-index job is already running for this scope")

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

// SeekStatus is the per-file diagnosis surfaced by Scan.
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

// SeekIndexService is the admin-facing seek-index maintenance use case.
type SeekIndexService struct {
	reindexer mkv.Reindexer
	catalog   MediaCatalog
	logger    *slog.Logger

	mu      sync.Mutex
	running map[string]bool // scope key ("*" or libraryID) → a batch job is in flight
}

func NewSeekIndexService(reindexer mkv.Reindexer, catalog MediaCatalog, logger *slog.Logger) *SeekIndexService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SeekIndexService{
		reindexer: reindexer,
		catalog:   catalog,
		logger:    logger.With("component", "seek_index"),
		running:   map[string]bool{},
	}
}

var ebmlExts = map[string]bool{".mkv": true, ".mka": true, ".mks": true, ".mk3d": true, ".webm": true}

func isEBMLPath(p string) bool { return ebmlExts[strings.ToLower(filepath.Ext(p))] }

func containerOf(p string) string { return strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".") }

// Scan inspects every EBML media file in the library (or all libraries when
// libraryID is empty) and reports its seek-index + backup status. Read-only.
func (s *SeekIndexService) Scan(ctx context.Context, libraryID string) (ScanResult, error) {
	files, err := s.catalog.ListMediaFiles(ctx, libraryID)
	if err != nil {
		return ScanResult{}, fmt.Errorf("seek-index scan: list media: %w", err)
	}
	var res ScanResult
	for _, f := range files {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if f.Path == "" || !isEBMLPath(f.Path) {
			continue
		}
		fi, statErr := os.Stat(f.Path)
		if statErr != nil {
			// File recorded in the DB but missing/unreadable on disk — skip it
			// rather than reporting a misleading "no index".
			s.logger.Warn("seek-index scan: file unreadable, skipping",
				slog.String("path", f.Path), slog.Any("err", statErr))
			continue
		}
		hasIdx, err := s.reindexer.HasSeekIndex(ctx, f.Path)
		if err != nil {
			s.logger.Warn("seek-index scan: parse failed, skipping",
				slog.String("path", f.Path), slog.Any("err", err))
			continue
		}
		hasBak, bakErr := s.reindexer.HasBackup(ctx, f.Path)
		if bakErr != nil {
			// Backup status is secondary diagnostics — a probe failure must not
			// drop the file from the scan, just report it as "no backup".
			s.logger.Warn("seek-index scan: backup probe failed",
				slog.String("path", f.Path), slog.Any("err", bakErr))
		}
		res.Items = append(res.Items, SeekStatus{
			MediaID:   f.MediaID,
			Title:     f.Title,
			Path:      f.Path,
			Container: containerOf(f.Path),
			HasIndex:  hasIdx,
			HasBackup: hasBak,
			SizeBytes: fi.Size(),
		})
		res.Total++
		if !hasIdx {
			res.MissingIndex++
		}
		if hasBak {
			res.WithBackup++
		}
	}
	return res, nil
}

// ReindexMedia rebuilds one media file's seek index (keeping a backup).
func (s *SeekIndexService) ReindexMedia(ctx context.Context, mediaID string) (mkv.ReindexResult, error) {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("seek-index reindex: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return mkv.ReindexResult{}, fmt.Errorf("seek-index reindex: media %s has no file", mediaID)
	}
	return s.reindexer.ReindexInPlace(ctx, f.Path, mkv.ReindexOptions{KeepBackup: true})
}

// RollbackMedia restores one media file from its backup.
func (s *SeekIndexService) RollbackMedia(ctx context.Context, mediaID string) error {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("seek-index rollback: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("seek-index rollback: media %s has no file", mediaID)
	}
	return s.reindexer.RestoreBackup(ctx, f.Path)
}

// ReindexLibrary reindexes every un-indexed file in a library in the background
// and returns the number of candidates queued. Progress is observable by
// re-running Scan (HasIndex flips to true). Refuses if a batch is already
// running for the scope.
func (s *SeekIndexService) ReindexLibrary(ctx context.Context, libraryID string) (int, error) {
	scan, err := s.Scan(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	todo := filterPaths(scan.Items, func(it SeekStatus) bool { return !it.HasIndex })
	return s.startBatch(libraryID, "reindex", todo, func(bg context.Context, path string) error {
		_, err := s.reindexer.ReindexInPlace(bg, path, mkv.ReindexOptions{KeepBackup: true})
		return err
	})
}

// RollbackLibrary restores every backed-up file in a library in the background.
func (s *SeekIndexService) RollbackLibrary(ctx context.Context, libraryID string) (int, error) {
	scan, err := s.Scan(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	todo := filterPaths(scan.Items, func(it SeekStatus) bool { return it.HasBackup })
	return s.startBatch(libraryID, "rollback", todo, func(bg context.Context, path string) error {
		return s.reindexer.RestoreBackup(bg, path)
	})
}

func filterPaths(items []SeekStatus, keep func(SeekStatus) bool) []string {
	var out []string
	for _, it := range items {
		if keep(it) {
			out = append(out, it.Path)
		}
	}
	return out
}

func (s *SeekIndexService) startBatch(libraryID, op string, paths []string, fn func(context.Context, string) error) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	key := jobKey(libraryID)
	if !s.tryStart(key) {
		return 0, ErrJobRunning
	}
	go s.runBatch(key, op, paths, fn)
	return len(paths), nil
}

func jobKey(libraryID string) string {
	if libraryID == "" {
		return "*"
	}
	return libraryID
}

func (s *SeekIndexService) tryStart(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A whole-catalog ("*") job conflicts with any per-library job and vice versa.
	if s.running[key] || s.running["*"] || (key != "*" && len(s.running) > 0) {
		return false
	}
	s.running[key] = true
	return true
}

func (s *SeekIndexService) finish(key string) {
	s.mu.Lock()
	delete(s.running, key)
	s.mu.Unlock()
}

// runBatch processes paths one at a time (sequential to bound disk/CPU on a weak
// host) in a fire-and-forget background goroutine. It uses a fresh context
// because it outlives the triggering request — the same pattern as the
// enrichment / album-art kickers. Each per-file failure leaves that file intact
// (the Reindexer/Restore are fail-safe) and the batch continues.
func (s *SeekIndexService) runBatch(key, op string, paths []string, fn func(context.Context, string) error) {
	defer s.finish(key)
	bg := context.Background()
	var done, failed int
	for _, p := range paths {
		if err := fn(bg, p); err != nil {
			failed++
			s.logger.Warn("seek-index "+op+" failed (file left intact)",
				slog.String("path", p), slog.Any("err", err))
			continue
		}
		done++
	}
	s.logger.Info("seek-index "+op+" batch complete",
		slog.String("scope", key), slog.Int("done", done),
		slog.Int("failed", failed), slog.Int("total", len(paths)))
}
