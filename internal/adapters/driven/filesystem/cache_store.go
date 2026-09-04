package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// cacheInfoTTL bounds how stale the admin cache page may be. GetCacheInfo
// walks every hash directory of the transcode cache recursively, which is the
// expensive part on the weak-NAS target this project aims at. The page is a
// one-shot load (no refetch), so this only ever coalesces a reload or two
// operators looking at once - and any change WE make invalidates immediately,
// so the only thing that can be stale is a segment the transcoder wrote on its
// own, which the next reload picks up.
const cacheInfoTTL = 10 * time.Second

type CacheStore struct {
	dir string

	mu     sync.Mutex
	memo   *transcoding.CacheInfo
	memoAt time.Time
	nowFn  func() time.Time
}

func NewCacheStore(dir string) *CacheStore {
	return &CacheStore{dir: dir, nowFn: time.Now}
}

func (s *CacheStore) now() time.Time {
	if s.nowFn == nil {
		return time.Now()
	}
	return s.nowFn()
}

// invalidate drops the memo. Called by every method that MUTATES the cache:
// an operator who clears the cache and still sees the old total would
// reasonably conclude the button is broken.
func (s *CacheStore) invalidate() {
	s.mu.Lock()
	s.memo = nil
	s.mu.Unlock()
}

func (s *CacheStore) cached() *transcoding.CacheInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memo == nil || s.now().Sub(s.memoAt) >= cacheInfoTTL {
		return nil
	}
	return s.memo
}

func (s *CacheStore) store(info *transcoding.CacheInfo) {
	s.mu.Lock()
	s.memo, s.memoAt = info, s.now()
	s.mu.Unlock()
}

func (s *CacheStore) GetCacheInfo(_ context.Context) (*transcoding.CacheInfo, error) {
	if hit := s.cached(); hit != nil {
		return hit, nil
	}
	info := &transcoding.CacheInfo{}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return nil, fmt.Errorf("read cache dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			subDir := filepath.Join(s.dir, e.Name())
			// Descend into subdirectories (e.g. hls/) to list individual sessions
			subEntries, subErr := os.ReadDir(subDir)
			if subErr == nil && len(subEntries) > 0 {
				for _, sub := range subEntries {
					if !sub.IsDir() {
						continue
					}
					size, latest := dirStats(filepath.Join(subDir, sub.Name()))
					if size == 0 {
						continue
					}
					info.Files = append(info.Files, transcoding.CacheFileInfo{
						Name:         sub.Name(),
						Size:         size,
						LastModified: latest,
					})
					info.TotalSize += size
					info.FileCount++
				}
				continue
			}
			size, latest := dirStats(subDir)
			if size == 0 {
				continue
			}
			info.Files = append(info.Files, transcoding.CacheFileInfo{
				Name:         e.Name(),
				Size:         size,
				LastModified: latest,
			})
			info.TotalSize += size
			info.FileCount++
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		info.Files = append(info.Files, transcoding.CacheFileInfo{
			Name:         fi.Name(),
			Size:         fi.Size(),
			LastModified: fi.ModTime(),
		})
		info.TotalSize += fi.Size()
		info.FileCount++
	}
	s.store(info)
	return info, nil
}

// dirStats walks a directory recursively and returns total size + latest mod time.
func dirStats(root string) (int64, time.Time) {
	var total int64
	var latest time.Time
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Debug("dirStats: walk error", slog.String("path", path), slog.Any("err", err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			slog.Debug("dirStats: stat error", slog.String("path", path), slog.Any("err", err))
			return nil
		}
		total += fi.Size()
		if fi.ModTime().After(latest) {
			latest = fi.ModTime()
		}
		return nil
	}); err != nil {
		slog.Warn("dirStats walk", slog.Any("err", err))
	}
	return total, latest
}

// ClearCache empties the cache directory.
//
// ⚠️ It deletes the bytes out from under whatever is writing them: a live
// transcode session keeps its ffmpeg writing into <dir>/segments/<hash>/, and
// that process dies on ENOENT mid-segment (`exit status 254`, a page of muxer
// rename failures) rather than on a clean kill. The orchestrator respawns and
// playback survives, so this is a rough edge, not data loss - but it means an
// admin clearing the cache during a playback WILL show up as errors in the log.
//
// Hence the Info line below, added 2026-09-04. Without it nothing anywhere
// recorded that a wipe happened, and a real incident (a segment directory
// vanishing 7 s after its creation, mid-playback) could not be told apart from
// a concurrency bug in the orchestrator without ruling out every other suspect
// first. A destructive admin action must leave a trace.
func (s *CacheStore) ClearCache(_ context.Context) error {
	defer s.invalidate()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cache dir: %w", err)
	}
	// Sized before the delete: afterwards there is nothing left to measure.
	freed, _ := dirStats(s.dir)
	removed, failed := 0, 0
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
			failed++
			slog.Warn("clear cache entry", slog.String("entry", e.Name()), slog.Any("err", err))
			continue
		}
		removed++
	}
	slog.Info("transcode cache cleared by admin",
		slog.Int("entries_removed", removed),
		slog.Int("entries_failed", failed),
		slog.Int64("freed_bytes", freed),
		slog.String("dir", s.dir))
	return nil
}

func (s *CacheStore) DeleteFile(_ context.Context, filename string) error {
	defer s.invalidate()
	if strings.Contains(filename, "..") {
		return errors.New("invalid filename")
	}
	// Try direct child first, then search in subdirs (e.g. hls/<mediaId>)
	path := filepath.Join(s.dir, filename)
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, filepath.Clean(s.dir)+string(os.PathSeparator)) {
		return errors.New("invalid filename")
	}
	if _, err := os.Stat(clean); os.IsNotExist(err) {
		// Search in subdirectories
		entries, _ := os.ReadDir(s.dir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(s.dir, e.Name(), filename)
			if _, statErr := os.Stat(candidate); statErr == nil {
				clean = candidate
				break
			}
		}
	}
	fi, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("file not found")
		}
		return fmt.Errorf("stat file: %w", err)
	}
	if fi.IsDir() {
		if err := os.RemoveAll(clean); err != nil {
			return fmt.Errorf("delete dir: %w", err)
		}
		return nil
	}
	if err := os.Remove(clean); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}
