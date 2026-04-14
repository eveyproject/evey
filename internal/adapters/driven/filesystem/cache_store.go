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
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

type CacheStore struct {
	dir string
}

func NewCacheStore(dir string) *CacheStore {
	return &CacheStore{dir: dir}
}

func (s *CacheStore) GetCacheInfo(_ context.Context) (*transcoding.CacheInfo, error) {
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

func (s *CacheStore) ClearCache(_ context.Context) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cache dir: %w", err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
			slog.Warn("clear cache entry", slog.Any("err", err))
		}
	}
	return nil
}

func (s *CacheStore) DeleteFile(_ context.Context, filename string) error {
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
