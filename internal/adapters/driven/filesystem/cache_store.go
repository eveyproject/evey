package filesystem

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
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
			size, latest := dirStats(filepath.Join(s.dir, e.Name()))
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
	if err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr
		}
		fi, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr
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
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return fmt.Errorf("invalid filename")
	}
	path := filepath.Join(s.dir, filename)
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, filepath.Clean(s.dir)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid filename")
	}
	fi, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found")
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
