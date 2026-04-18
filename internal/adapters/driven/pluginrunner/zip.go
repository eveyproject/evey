package pluginrunner

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxZipFileSize      = 100 * 1024 * 1024  // 100 MB per file
	maxZipFiles         = 5000               // entry count cap
	maxZipTotalBytes    = 500 * 1024 * 1024  // 500 MB cumulated uncompressed
	maxCompressionRatio = 100                // compressed:uncompressed — detect zip-bombs
	safeFileModeMask    = os.FileMode(0o755) // strip setuid/setgid/sticky
)

type ZipInstaller struct{}

func NewZipInstaller() *ZipInstaller { return &ZipInstaller{} }

func (z *ZipInstaller) Extract(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close() //nolint:errcheck

	if len(r.File) > maxZipFiles {
		return fmt.Errorf("zip exceeds max files: %d > %d", len(r.File), maxZipFiles)
	}

	destDir = filepath.Clean(destDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	var totalBytes uint64
	for _, f := range r.File {
		if err := extractZipFile(f, destDir, &totalBytes); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, destDir string, totalBytes *uint64) error {
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

	// zip-bomb: compression ratio > maxCompressionRatio triggers refusal.
	if f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > maxCompressionRatio {
		return fmt.Errorf("file %s suspicious compression ratio: %d:1", f.Name, f.UncompressedSize64/f.CompressedSize64)
	}

	if *totalBytes+f.UncompressedSize64 > maxZipTotalBytes {
		return fmt.Errorf("zip exceeds max total size: %d > %d bytes", *totalBytes+f.UncompressedSize64, maxZipTotalBytes)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	// Strip setuid/setgid/sticky bits; keep rwx.
	mode := f.Mode() & safeFileModeMask
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create file %s: %w", target, err)
	}
	n, err := io.Copy(out, io.LimitReader(rc, int64(maxZipFileSize)+1))
	if err != nil {
		_ = out.Close()
		return fmt.Errorf("write file %s: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close file %s: %w", target, err)
	}
	*totalBytes += uint64(n)
	return nil
}
