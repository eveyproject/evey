package filesystem

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/backup"
)

type BackupStore struct {
	dir string
}

func NewBackupStore(dir string) *BackupStore {
	return &BackupStore{dir: dir}
}

func (s *BackupStore) Create(_ context.Context, m *backup.BackupManifest, data io.Reader) error {
	if err := os.MkdirAll(s.dir, 0750); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	archivePath := filepath.Join(s.dir, m.ID+".gz")
	f, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer func() {
		if closeErr := gw.Close(); closeErr != nil {
			slog.Warn("close gzip writer", slog.Any("err", closeErr))
		}
	}()

	manifestBytes, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	// write manifest length prefix + manifest + dump data
	header := fmt.Sprintf("%d\n", len(manifestBytes))
	if _, err2 := gw.Write([]byte(header)); err2 != nil {
		return err2
	}
	if _, err2 := gw.Write(manifestBytes); err2 != nil {
		return err2
	}
	written, err := io.Copy(gw, data)
	if err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	if err2 := gw.Close(); err2 != nil {
		return err2
	}
	if err2 := f.Close(); err2 != nil {
		return err2
	}

	fi, err := os.Stat(archivePath)
	if err == nil {
		m.Size = fi.Size()
	} else {
		m.Size = written
	}

	// write manifest sidecar for fast listing
	manifestPath := filepath.Join(s.dir, m.ID+".json")
	return os.WriteFile(manifestPath, manifestBytes, 0640)
}

func (s *BackupStore) List(_ context.Context) ([]*backup.BackupManifest, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	var result []*backup.BackupManifest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var m backup.BackupManifest
		if json.Unmarshal(data, &m) == nil {
			// update size from archive
			archivePath := filepath.Join(s.dir, m.ID+".gz")
			if fi, err := os.Stat(archivePath); err == nil {
				m.Size = fi.Size()
			}
			result = append(result, &m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (s *BackupStore) Get(_ context.Context, id string) (*backup.BackupManifest, io.ReadCloser, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return nil, nil, fmt.Errorf("invalid backup id")
	}
	manifestPath := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("backup not found")
	}
	var m backup.BackupManifest
	if err2 := json.Unmarshal(data, &m); err2 != nil {
		return nil, nil, fmt.Errorf("corrupt manifest")
	}
	archivePath := filepath.Join(s.dir, id+".gz")
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, nil, fmt.Errorf("backup archive not found")
	}
	if fi, err2 := os.Stat(archivePath); err2 == nil {
		m.Size = fi.Size()
	}
	return &m, f, nil
}

func (s *BackupStore) Delete(_ context.Context, id string) error {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid backup id")
	}
	archivePath := filepath.Join(s.dir, id+".gz")
	manifestPath := filepath.Join(s.dir, id+".json")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		return fmt.Errorf("backup not found")
	}
	if err := os.Remove(archivePath); err != nil {
		slog.Warn("remove backup archive", slog.Any("err", err))
	}
	if err := os.Remove(manifestPath); err != nil {
		slog.Warn("remove backup manifest", slog.Any("err", err))
	}
	return nil
}
