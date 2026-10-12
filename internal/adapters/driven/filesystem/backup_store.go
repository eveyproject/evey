package filesystem

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eveyproject/evey/internal/domain/backup"
)

type BackupStore struct {
	dir string
}

func NewBackupStore(dir string) *BackupStore {
	return &BackupStore{dir: dir}
}

// resolveArchivePath returns the path to the archive file, trying .tar.gz first then legacy .gz.
func (s *BackupStore) resolveArchivePath(id string) (string, bool) {
	tarPath := filepath.Join(s.dir, id+".tar.gz")
	if _, err := os.Stat(tarPath); err == nil {
		return tarPath, true
	}
	gzPath := filepath.Join(s.dir, id+".gz")
	if _, err := os.Stat(gzPath); err == nil {
		return gzPath, true
	}
	return tarPath, false
}

func (s *BackupStore) Create(_ context.Context, m *backup.BackupManifest, data io.Reader) error {
	// Owner-only: a DB backup is the whole database (incl. password hashes). On
	// kernels without Landlock a plugin runs DAC-isolated from the server's uid,
	// so 0700 dir + 0600 files keep backups unreadable to it (belt-and-suspenders
	// alongside the chmod 700 in the image).
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	archivePath := filepath.Join(s.dir, m.ID+".tar.gz")
	f, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	manifestBytes, marshalErr := json.Marshal(m)
	if marshalErr != nil {
		return fmt.Errorf("marshal manifest: %w", marshalErr)
	}
	if writeErr := writeTarEntry(tw, "manifest.json", manifestBytes); writeErr != nil {
		return fmt.Errorf("write manifest: %w", writeErr)
	}

	if writeErr := streamTarEntry(tw, "dump.sql", data); writeErr != nil {
		return fmt.Errorf("write dump: %w", writeErr)
	}

	if closeErr := tw.Close(); closeErr != nil {
		return closeErr
	}
	if closeErr := gw.Close(); closeErr != nil {
		return closeErr
	}
	if closeErr := f.Close(); closeErr != nil {
		return closeErr
	}

	fi, err := os.Stat(archivePath)
	if err == nil {
		m.Size = fi.Size()
	}

	// sidecar for fast listing
	sidecarPath := filepath.Join(s.dir, m.ID+".json")
	return os.WriteFile(sidecarPath, manifestBytes, 0600)
}

func streamTarEntry(tw *tar.Writer, name string, r io.Reader) error {
	var size int64
	var src io.Reader

	if rs, ok := r.(io.ReadSeeker); ok {
		cur, err := rs.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		end, err := rs.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
		size = end - cur
		if _, err := rs.Seek(cur, io.SeekStart); err != nil {
			return err
		}
		src = rs
	} else {
		tmp, err := os.CreateTemp("", "evey-tar-entry-*")
		if err != nil {
			return fmt.Errorf("create temp: %w", err)
		}
		defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup
		defer tmp.Close()

		size, err = io.Copy(tmp, r)
		if err != nil {
			return fmt.Errorf("buffer to temp: %w", err)
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		src = tmp
	}

	// 0600: the SQL dump carries bcrypt hashes + DB-stored secrets (SMTP/VAPID/
	// OIDC/TIDB), so keep extracted files owner-only, matching the 0600 archive.
	hdr := &tar.Header{Name: name, Mode: 0600, Size: size}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := io.Copy(tw, src)
	return err
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name: name,
		Mode: 0600,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
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
		data, readErr := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if readErr != nil {
			continue
		}
		var m backup.BackupManifest
		if json.Unmarshal(data, &m) == nil {
			if archivePath, ok := s.resolveArchivePath(m.ID); ok {
				if fi, statErr := os.Stat(archivePath); statErr == nil {
					m.Size = fi.Size()
				}
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
		return nil, nil, errors.New("invalid backup id")
	}
	manifestPath := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, backup.ErrNotFound
		}
		return nil, nil, fmt.Errorf("read backup manifest: %w", err)
	}
	var m backup.BackupManifest
	if unmarshalErr := json.Unmarshal(data, &m); unmarshalErr != nil {
		return nil, nil, errors.New("corrupt manifest")
	}
	archivePath, ok := s.resolveArchivePath(id)
	if !ok {
		return nil, nil, backup.ErrNotFound
	}
	f, openErr := os.Open(archivePath)
	if openErr != nil {
		if errors.Is(openErr, os.ErrNotExist) {
			return nil, nil, backup.ErrNotFound
		}
		return nil, nil, fmt.Errorf("open backup archive: %w", openErr)
	}
	if fi, statErr := os.Stat(archivePath); statErr == nil {
		m.Size = fi.Size()
	}
	return &m, f, nil
}

func (s *BackupStore) Delete(_ context.Context, id string) error {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return errors.New("invalid backup id")
	}
	archivePath, ok := s.resolveArchivePath(id)
	if !ok {
		return backup.ErrNotFound
	}
	manifestPath := filepath.Join(s.dir, id+".json")
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove backup archive: %w", err)
	}
	if err := os.Remove(manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove backup manifest: %w", err)
	}
	return nil
}
