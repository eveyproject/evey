package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
// whose title was written by an older parser version). It is the file
// name's title, not the season-folder layout's show title: the heal only
// rewrites movies, and a movie is named by its file. Pure function -
// stat-less, idempotent, no I/O.
func (s *Scanner) ParseTitle(path string) string {
	return ParseFilename(path).Title
}

// ParseYear is the year Scan stores for path: the file name's, else its
// folder's ("Show Name (2025)/"). Pure, no I/O.
func (s *Scanner) ParseYear(path string) *int {
	return parseScanned(path).Year
}

// parseScanned is the one parse Scan, ParseTitle and ParseYear share, so a
// title healed later matches the title the scan wrote. The year falls back
// on the file's own folder ("Show Name (2025)/") when neither the name nor
// a season layout states one.
func parseScanned(path string) ParsedInfo {
	parsed := ParsePath(path)
	if parsed.Year == nil {
		parsed.Year = ParseFilename(filepath.Dir(path) + ".mkv").Year
	}
	return parsed
}

// FileExists reports whether path is still present on disk.
func (s *Scanner) FileExists(path string) bool {
	return notProvenGone(path)
}

// notProvenGone reports false only when the OS says the file does not exist.
// The scan prunes on this answer, and a stat that fails for any other reason
// (a folder whose permissions changed, EIO, a stale NFS handle) says nothing
// about the file being deleted: reading it as gone would drop the item and
// its watch history.
func notProvenGone(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

func NewScanner() *Scanner {
	return &Scanner{}
}

// errRootNotDir is a library root that resolves to something other than a
// folder: there is nothing to walk, and walking it as one file read as an
// empty mount.
var errRootNotDir = errors.New("library root is not a directory")

// walkLibraryRoot is filepath.WalkDir with the root resolved through any
// symbolic link first: WalkDir never follows a link, so a linked root was
// visited as one file and yielded nothing, which the scan took for an empty
// mount. A root that is not a folder is reported to fn as the root's error,
// the same way a missing one is. Paths handed to fn keep the configured root
// as their prefix: the catalog rows, the pruner's roots and the watcher are
// all keyed on it, not on where the link points.
func walkLibraryRoot(root string, fn fs.WalkDirFunc) error {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fn(root, nil, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fn(root, nil, err)
	}
	if !info.IsDir() {
		return fn(root, nil, &fs.PathError{Op: "scan", Path: root, Err: errRootNotDir})
	}
	if resolved == root {
		return filepath.WalkDir(root, fn)
	}
	return filepath.WalkDir(resolved, func(path string, d fs.DirEntry, walkErr error) error {
		rel, relErr := filepath.Rel(resolved, path)
		if relErr != nil || rel == "." {
			return fn(root, d, walkErr)
		}
		return fn(filepath.Join(root, rel), d, walkErr)
	})
}

func (s *Scanner) Scan(ctx context.Context, root string) ([]library.ScannedFile, error) {
	ctx, span := tracing.StartSpan(ctx, "filesystem.scanner.Scan")
	defer span.End()

	var files []library.ScannedFile
	err := walkLibraryRoot(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			// Skipped, not fatal: the pruner re-checks each unseen file on disk,
			// so what lives under this folder keeps its item.
			slog.Warn("scan: walk error", slog.String("path", path), slog.Any("err", err))
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
		parsed := parseScanned(path)
		if parsed.Extra == "sample" {
			slog.Debug("scan: skipping sample file", slog.String("path", path))
			return nil
		}
		itemType := library.MovieItem
		if parsed.Type == "series" {
			itemType = library.Episode
		}
		size := info.Size()
		modTime := info.ModTime()
		files = append(files, library.ScannedFile{
			Path:         path,
			Size:         size,
			ModTime:      modTime,
			Fingerprint:  fmt.Sprintf("%d-%d", size, modTime.UnixNano()),
			Title:        parsed.Title,
			Type:         itemType,
			Year:         parsed.Year,
			Season:       parsed.Season,
			Episode:      parsed.Episode,
			FolderSeason: parsed.FolderSeason,
			NameTitle:    parsed.NameTitle,
			Extra:        parsed.Extra,
		})
		return nil
	})
	return files, err
}
