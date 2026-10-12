package filesystem

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	".ts": true, ".m2ts": true, ".mpg": true, ".mpeg": true,
	".3gp": true, ".ogv": true, ".vob": true,
}

// looksLikeMPEGTS sniffs the first 5 bytes to validate that a `.ts` or
// `.m2ts` file is an actual MPEG transport stream rather than a TypeScript
// source file (or anything else) that happens to share the extension.
// MPEG-TS packets are 188 bytes and start with sync byte 0x47; M2TS adds
// a 4-byte timestamp prefix so the sync byte sits at offset 4.
func looksLikeMPEGTS(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var buf [5]byte
	n, _ := io.ReadFull(f, buf[:])
	if n < 1 {
		return false
	}
	if buf[0] == 0x47 {
		return true
	}
	return n >= 5 && buf[4] == 0x47
}

type Scanner struct{}

// ParseTitle exposes the filename parser used by the scanner so the app
// layer can re-derive a title from a known path (e.g. heal an orphan
// whose title was written by an older parser version). Pure function -
// stat-less, idempotent, no I/O.
func (s *Scanner) ParseTitle(path string) string {
	return (&FilenameParser{}).Parse(path).Title
}

// FileExists reports whether path is still present on disk.
func (s *Scanner) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func NewScanner() *Scanner {
	return &Scanner{}
}

func (s *Scanner) Scan(ctx context.Context, root string) ([]library.ScannedFile, error) {
	ctx, span := tracing.StartSpan(ctx, "filesystem.scanner.Scan")
	defer span.End()

	var files []library.ScannedFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		name := d.Name()

		if d.IsDir() {
			if name != "." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasPrefix(name, ".") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if !videoExtensions[ext] {
			return nil
		}

		if (ext == ".ts" || ext == ".m2ts") && !looksLikeMPEGTS(path) {
			slog.Debug("scan: skipping non-MPEG-TS file with .ts extension", slog.String("path", path))
			return nil
		}

		info, err := d.Info()
		if err != nil {
			slog.Debug("scan: stat error", slog.String("path", path), slog.Any("err", err))
			return nil
		}
		parsed := (&FilenameParser{}).Parse(path)
		// If no year from filename, try parent directory (e.g. "Show Name (2025)/")
		if parsed.Year == nil {
			dirParsed := (&FilenameParser{}).Parse(filepath.Dir(path) + ".mkv")
			if dirParsed.Year != nil {
				parsed.Year = dirParsed.Year
			}
		}
		itemType := library.MovieItem
		if parsed.Type == "series" {
			itemType = library.Episode
		}
		size := info.Size()
		modTime := info.ModTime()
		files = append(files, library.ScannedFile{
			Path:        path,
			Size:        size,
			ModTime:     modTime,
			Fingerprint: fmt.Sprintf("%d-%d", size, modTime.UnixNano()),
			Title:       parsed.Title,
			Type:        itemType,
			Year:        parsed.Year,
			Season:      parsed.Season,
			Episode:     parsed.Episode,
		})
		return nil
	})
	return files, err
}
