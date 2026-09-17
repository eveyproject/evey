package maintenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"log/slog"
)

// MediaRelocator repoints a catalog entry at a file that moved on disk. The
// wrong-container repair renames the file itself and needs the library to
// follow, so playback and the next scan see one file, not a vanished .mkv
// plus an unknown .mp4.
type MediaRelocator interface {
	RelocateMediaFile(ctx context.Context, mediaID, newPath string) error
}

// WithMediaRelocator enables the wrong-container repair (rename to .mp4 and
// repoint the catalog). Without it, such files stay a manual chore.
func (s *SeekIndexService) WithMediaRelocator(r MediaRelocator) *SeekIndexService {
	s.relocator = r
	return s
}

// SupportsContainerFix reports whether the wrong-container repair is wired.
func (s *SeekIndexService) SupportsContainerFix() bool { return s.relocator != nil }

// ErrContainerFixUnavailable: no relocator wired.
var ErrContainerFixUnavailable = errors.New("seek-index: container fix not available on this server")

// StartFixContainerMedia renames one mislabelled .mkv to .mp4 in the
// background and repoints the catalog. The media lookup is synchronous so an
// unknown ID fails fast; the file must carry the WrongContainer verdict in a
// cached scan - the repair renames on the strength of that diagnosis alone.
func (s *SeekIndexService) StartFixContainerMedia(ctx context.Context, mediaID string) error {
	if s.relocator == nil {
		return ErrContainerFixUnavailable
	}
	ref, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("seek-index: media %s: %w", mediaID, err)
	}
	st, ok := s.cachedStatusByPath(ref.Path)
	if !ok || !st.WrongContainer {
		return fmt.Errorf("seek-index: %s is not diagnosed as a wrong container: %w", ref.Path, ErrAckNotApplicable)
	}
	return s.startSingle(ctx, JobFixContainer, mediaID, func(bg context.Context, path string) (bool, error) {
		return s.fixContainerStep(bg, path)
	})
}

// fixContainerStep renames path to the .mp4 extension and repoints the
// catalog entry. The rename is atomic on one filesystem; if the catalog
// refuses the new path the file is renamed back so disk and database never
// disagree. On success the file leaves the cached scan: it is no longer an
// EBML file and the next scan will not list it.
func (s *SeekIndexService) fixContainerStep(bg context.Context, path string) (bool, error) {
	st, err := s.freshStatusForRepair(bg, path)
	if err != nil {
		return false, err
	}
	if !st.WrongContainer {
		return true, nil
	}
	newPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".mp4"
	if _, err := os.Stat(newPath); err == nil {
		return false, fmt.Errorf("seek-index: %s already exists, not overwriting", newPath)
	}
	if err := os.Rename(path, newPath); err != nil {
		return false, fmt.Errorf("seek-index: rename to .mp4: %w", err)
	}
	if err := s.relocator.RelocateMediaFile(bg, st.MediaID, newPath); err != nil {
		if back := os.Rename(newPath, path); back != nil {
			s.logger.Error("seek-index: catalog refused the new path AND the rename back failed - disk and catalog disagree",
				slog.String("path", path), slog.String("new_path", newPath), slog.Any("relocate_err", err), slog.Any("rename_back_err", back))
		}
		return false, fmt.Errorf("seek-index: repoint catalog to %s: %w", newPath, err)
	}
	s.dropCachedFile(path)
	s.saveScans()
	s.logger.Info("seek-index: container fixed - mislabelled .mkv renamed",
		slog.String("path", path), slog.String("new_path", newPath))
	return false, nil
}

// freshStatusForRepair is the invariant every repair that acts on a CACHED
// verdict goes through: the status is only trusted while the file still has
// the (size, mtime) fingerprint the diagnosis was made on, under the current
// detectors. A file replaced since the scan (an *arr upgrade, a re-acquire) is
// re-diagnosed on the spot and the cache updated, so the repair judges the
// bytes on disk - a rename or a re-mux on the strength of a verdict about a
// file that no longer exists is not a repair. Nothing here can be cheap for a
// stale file: the re-probe is the price of acting on it at all.
func (s *SeekIndexService) freshStatusForRepair(ctx context.Context, path string) (SeekStatus, error) {
	cached, ok := s.cachedStatusByPath(path)
	if !ok {
		return SeekStatus{}, fmt.Errorf("%w: %s", ErrNotInScan, path)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return SeekStatus{}, fmt.Errorf("seek-index: %s vanished since its diagnosis: %w", path, err)
	}
	if !cached.Unreadable && cached.DiagVersion == SeekDiagVersion &&
		cached.SizeBytes == fi.Size() && cached.ModTimeUnix == fi.ModTime().Unix() {
		return cached, nil
	}
	s.logger.Info("seek-index: file changed since its diagnosis, re-probing before the repair",
		slog.String("path", path), slog.Int64("cached_size", cached.SizeBytes), slog.Int64("size", fi.Size()))
	fresh, _, ok := s.probeFile(ctx, MediaRef{MediaID: cached.MediaID, Title: cached.Title, Path: path}, nil)
	if !ok {
		if ctx.Err() != nil {
			return SeekStatus{}, ctx.Err()
		}
		return SeekStatus{}, fmt.Errorf("%w: %s could not be re-diagnosed", ErrDiagnosisStale, path)
	}
	if fresh.Unreadable {
		return SeekStatus{}, fmt.Errorf("%w: %s: %s", ErrDiagnosisStale, path, fresh.UnreadableDetail)
	}
	s.updateCachedFile(path, func(c *SeekStatus) { *c = fresh })
	s.saveScans()
	return fresh, nil
}

// cachedStatusByPath finds a file in any cached scope.
func (s *SeekIndexService) cachedStatusByPath(path string) (SeekStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.scans {
		for i := range c.res.Items {
			if c.res.Items[i].Path == path {
				return c.res.Items[i], true
			}
		}
	}
	return SeekStatus{}, false
}

// dropCachedFile removes a file from every cached scope and recomputes the
// rollups - for a file that stopped being the scan's business.
func (s *SeekIndexService) dropCachedFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, c := range s.scans {
		kept := c.res.Items[:0]
		for _, it := range c.res.Items {
			if it.Path != path {
				kept = append(kept, it)
			}
		}
		if len(kept) == len(c.res.Items) {
			continue
		}
		c.res.Items = kept
		c.res.Total = len(kept)
		recomputeRollups(&c.res)
		s.scans[key] = c
	}
}
