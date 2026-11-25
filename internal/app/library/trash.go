package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// TrashDirName is the folder, at the root of the library that holds a file,
// where TrashMediaItem moves it. The scanner and the watcher skip every
// dot-folder, so nothing in it is indexed again. Emptying it is the
// administrator's call, by hand.
const TrashDirName = ".evey-trash"

// rename is os.Rename; tests swap it to stand in for two disks.
var rename = os.Rename

// partialSuffix marks a copy still being written into the trash.
const partialSuffix = ".evey-partial"

// ErrTrashNoFile is a media item with no file of its own: a series, a season
// or an album is a folder, never moved to the trash as a whole.
var ErrTrashNoFile = errors.New("media item has no file of its own")

// ErrTrashOutsideLibrary is a file that sits under none of its library's
// roots: there is no trash to move it to, and moving it anywhere else would
// write outside what the administrator pointed Evey at.
var ErrTrashOutsideLibrary = errors.New("media file is outside its library's folders")

// TrashMediaItem moves one media file, with the files beside it that share
// its name (subtitles, .nfo), to <library root>/.evey-trash/<same relative
// path>, then removes the item from Evey the way an admin delete does. The
// file is moved, never deleted: getting it back is a move in the other
// direction and a scan. When the trash folder is on another disk than the
// file (a pooled library), the move is a copy then a removal, as mv does. It
// returns where the file now is.
//
// The move comes first: if it fails (a read-only mount, a file already gone)
// the item stays in Evey untouched.
func (s *CommandService) TrashMediaItem(ctx context.Context, id string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.TrashMediaItem")
	defer span.End()
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(id))
	if err != nil {
		return "", fmt.Errorf("CommandService.TrashMediaItem: %w", err)
	}
	if !item.IsPlayable() {
		return "", fmt.Errorf("CommandService.TrashMediaItem: %w", ErrTrashNoFile)
	}
	lib, err := s.scan.libQueries.GetByID(ctx, item.LibraryID)
	if err != nil {
		return "", fmt.Errorf("CommandService.TrashMediaItem: library: %w", err)
	}
	root := libraryRootOf(lib.Paths, item.File.Path)
	if root == "" {
		return "", fmt.Errorf("CommandService.TrashMediaItem: %w", ErrTrashOutsideLibrary)
	}
	// Across disks the move is a copy, minutes for a large film: once begun it
	// finishes, and the item is removed, even if the caller hangs up.
	ctx = context.WithoutCancel(ctx)
	dest, err := moveToTrash(root, item.File.Path)
	if err != nil {
		return "", fmt.Errorf("CommandService.TrashMediaItem: %w", err)
	}
	if err := s.DeleteMediaItem(ctx, id); err != nil {
		// The file is in the trash and the row still points at its old place:
		// the next scan sees it missing and offers to remove it. Say so.
		slog.ErrorContext(ctx, "library: file moved to the trash but the item could not be removed",
			slog.String("mediaItemID", id), slog.String("trashed", dest), slog.Any("err", err))
		return dest, fmt.Errorf("CommandService.TrashMediaItem: %w", err)
	}
	slog.InfoContext(ctx, "library: media file moved to the trash",
		slog.String("mediaItemID", id), slog.String("from", item.File.Path), slog.String("to", dest))
	return dest, nil
}

// libraryRootOf returns the library root the path lies under, the longest
// one when roots nest; "" when none holds it.
func libraryRootOf(roots []string, path string) string {
	best := ""
	clean := filepath.Clean(path)
	for _, r := range roots {
		r = filepath.Clean(r)
		if r == "/" || r == "." {
			continue
		}
		if strings.HasPrefix(clean, r+string(filepath.Separator)) && len(r) > len(best) {
			best = r
		}
	}
	return best
}

// moveToTrash renames the file and its same-name companions into the trash
// under root, keeping their path relative to root. A name already taken in
// the trash (the same title trashed twice) gets a timestamp, never overwrites.
func moveToTrash(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", ErrTrashOutsideLibrary
	}
	destDir := filepath.Join(root, TrashDirName, filepath.Dir(rel))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create trash folder: %w", err)
	}
	dest := freeName(filepath.Join(destDir, filepath.Base(path)))
	if err := moveFile(path, dest); err != nil {
		return "", fmt.Errorf("move to trash: %w", err)
	}
	moveCompanions(path, destDir)
	return dest, nil
}

// moveCompanions moves the subtitles and .nfo named after the video at path
// ("Film.mkv" -> "Film.fr.srt", "Film.nfo") into destDir. Only those
// extensions: another copy of the same film in the same folder
// ("Film.2021.mkv") shares the prefix and must stay. Best-effort: the video
// is what frees the space.
func moveCompanions(path, destDir string) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, base+".") || !companionExt[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		from := filepath.Join(filepath.Dir(path), name)
		if mvErr := moveFile(from, freeName(filepath.Join(destDir, name))); mvErr != nil {
			slog.Warn("library: companion file left in place", slog.String("file", from), slog.Any("err", mvErr))
		}
	}
}

// companionExt lists what may follow a video into the trash.
var companionExt = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true,
	".idx": true, ".sup": true, ".nfo": true,
}

// moveFile moves a file as mv does. A rename when it can; when the two places
// are on different disks (a mergerfs or unRAID pool, two mounts) rename(2)
// refuses with EXDEV, and the file is copied then removed: written under a
// partial name next to its destination, flushed to disk, its size checked,
// renamed into place, and only then the original removed. A copy that fails
// leaves the original untouched and no partial file behind.
func moveFile(from, to string) error {
	err := rename(from, to)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	return copyThenRemove(from, to)
}

func copyThenRemove(from, to string) (err error) {
	src, err := os.Open(from) //nolint:gosec // G304 FP: a library media file the trash already resolved under its library root
	if err != nil {
		return err
	}
	defer src.Close() //nolint:errcheck // read-only
	info, err := src.Stat()
	if err != nil {
		return err
	}
	partial := to + partialSuffix
	dst, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()) //nolint:gosec // G304 FP: next to a destination under the library root
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = dst.Close()
			_ = os.Remove(partial)
		}
	}()
	n, err := io.CopyBuffer(dst, src, make([]byte, 4<<20))
	if err != nil {
		return fmt.Errorf("copy across disks: %w", err)
	}
	if n != info.Size() {
		return fmt.Errorf("copy across disks: wrote %d of %d bytes", n, info.Size())
	}
	if err = dst.Sync(); err != nil {
		return fmt.Errorf("copy across disks: %w", err)
	}
	if err = dst.Close(); err != nil {
		return fmt.Errorf("copy across disks: %w", err)
	}
	_ = os.Chtimes(partial, info.ModTime(), info.ModTime())
	if err = rename(partial, to); err != nil {
		return err
	}
	if rmErr := os.Remove(from); rmErr != nil {
		// The copy is whole and in place; the original could not go. Leave
		// both rather than lose either, and say so.
		slog.Warn("library: moved by copy, the original could not be removed", slog.String("file", from), slog.Any("err", rmErr))
	}
	return nil
}

func freeName(p string) string {
	if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(p)
	return strings.TrimSuffix(p, ext) + "." + time.Now().UTC().Format("20060102T150405") + ext
}

// IsTrashError reports the refusals an administrator can act on, as opposed
// to an internal failure.
func IsTrashError(err error) bool {
	return errors.Is(err, ErrTrashNoFile) || errors.Is(err, ErrTrashOutsideLibrary) || errors.Is(err, ErrRestoreNotInTrash) ||
		errors.Is(err, ErrRestoreTaken) || errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist)
}

// ErrRestoreNotInTrash is a path that is not inside the trash of one of the
// libraries: restoring only ever moves a file out of .evey-trash.
var ErrRestoreNotInTrash = errors.New("path is not in a library trash")

// ErrRestoreTaken is a restore whose original place is occupied again.
var ErrRestoreTaken = errors.New("a file already exists where it was")

// RestoreTrashedFile undoes TrashMediaItem: it moves a trashed file, and its
// subtitles and .nfo, back to where it was, then scans that library in the
// background so the title comes back. It comes back as a new entry, matched
// again from its name; the play state of the old entry is not restored. It
// returns the file's restored path and the library to watch.
func (s *CommandService) RestoreTrashedFile(ctx context.Context, trashedPath string) (string, string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.RestoreTrashedFile")
	defer span.End()
	libs, err := s.scan.libQueries.List(ctx, library.ListLibrariesFilter{})
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RestoreTrashedFile: %w", err)
	}
	dest, libID, err := restoreTarget(libs, trashedPath)
	if err != nil {
		return "", "", err
	}
	clean := trashedPath
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", "", fmt.Errorf("CommandService.RestoreTrashedFile: %w", err)
	}
	if err := moveFile(clean, dest); err != nil {
		return "", "", fmt.Errorf("CommandService.RestoreTrashedFile: %w", err)
	}
	moveCompanions(clean, filepath.Dir(dest))
	slog.InfoContext(ctx, "library: media file restored from the trash",
		slog.String("from", clean), slog.String("to", dest), slog.String("libraryID", libID))
	scanCtx := context.WithoutCancel(ctx)
	go func() {
		if _, err := s.ScanLibraryReport(scanCtx, libID); err != nil {
			slog.WarnContext(scanCtx, "library: scan after a restore failed", slog.String("libraryID", libID), slog.Any("err", err))
		}
	}()
	return dest, libID, nil
}

// restoreTarget says where a trashed file goes back to and which library it
// belongs to: the path must be clean, absolute and inside the .evey-trash of
// one of the libraries' roots, and its original place must be free.
func restoreTarget(libs []*library.Library, trashedPath string) (string, string, error) {
	clean := filepath.Clean(trashedPath)
	if !filepath.IsAbs(clean) || clean != trashedPath {
		return "", "", ErrRestoreNotInTrash
	}
	var root, libID string
	for _, l := range libs {
		if r := libraryRootOf(l.Paths, clean); r != "" && len(r) > len(root) {
			root, libID = r, string(l.ID)
		}
	}
	if root == "" {
		return "", "", ErrRestoreNotInTrash
	}
	rel, err := filepath.Rel(filepath.Join(root, TrashDirName), clean)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", "", ErrRestoreNotInTrash
	}
	dest := filepath.Join(root, rel)
	if _, err := os.Lstat(dest); err == nil {
		return "", "", ErrRestoreTaken
	}
	return dest, libID, nil
}
