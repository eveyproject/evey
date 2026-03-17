package pluginrunner

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxZipFileSize = 100 * 1024 * 1024 // 100 MB

type ZipInstaller struct{}

func NewZipInstaller() *ZipInstaller { return &ZipInstaller{} }

func (z *ZipInstaller) Extract(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close() //nolint:errcheck

	destDir = filepath.Clean(destDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	for _, f := range r.File {
		if err := extractZipFile(f, destDir); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, destDir string) error {
	target := filepath.Join(destDir, f.Name) //nolint:gosec
	target = filepath.Clean(target)

	if !strings.HasPrefix(target, destDir+string(filepath.Separator)) && target != destDir {
		return fmt.Errorf("path traversal detected: %s", f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}

	if f.UncompressedSize64 > maxZipFileSize {
		return fmt.Errorf("file %s exceeds max size (%d bytes)", f.Name, maxZipFileSize)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return fmt.Errorf("create file %s: %w", target, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, io.LimitReader(rc, int64(maxZipFileSize)+1)); err != nil {
		return fmt.Errorf("write file %s: %w", target, err)
	}
	return nil
}
