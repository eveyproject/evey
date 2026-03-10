package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func (s *CacheStore) ClearCache(_ context.Context) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cache dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		os.Remove(filepath.Join(s.dir, e.Name()))
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
	if err := os.Remove(clean); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found")
		}
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}
