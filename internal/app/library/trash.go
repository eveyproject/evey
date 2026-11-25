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

// TrashResult is where a trashed file is, or will be. Pending means the
// file had to be copied to another disk and the copy runs in the background:
// the item leaves Evey when it is whole, and a MediaTrashFinished event says
// how it ended.
type TrashResult struct {
	TrashedPath string
	Pending     bool
}

// trashJob is a move to the trash that must copy across disks.
type trashJob struct {
	id, title, from, dest, destDir string
}

// TrashMediaItem moves one media file, with the files beside it that share
// its name (subtitles, .nfo), to <library root>/.evey-trash/<same relative
// path>, then removes the item from Evey the way an admin delete does. The
// file is moved, never deleted: getting it back is a move in the other
// direction and a scan.
//
// A move within one disk is a rename, done before this returns. When the
// trash folder is on another disk than the file (a pooled library) the move
// is a copy then a removal, as mv does, minutes for a large film: it is
// queued, this returns at once with Pending, and the background worker
// copies one file at a time. Asked again while its copy waits or runs, the
// same item answers the same pending move.
//
// Until the file is whole in the trash the item stays in Evey untouched; a
// move that fails (a read-only mount, a full disk) leaves both in place.
func (s *CommandService) TrashMediaItem(ctx context.Context, id string) (TrashResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.TrashMediaItem")
	defer span.End()
	if dest, ok := s.trashPending.Load(id); ok {
		return TrashResult{TrashedPath: dest.(string), Pending: true}, nil
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(id))
	if err != nil {
		return TrashResult{}, fmt.Errorf("CommandService.TrashMediaItem: %w", err)
	}
	if !item.IsPlayable() {
		return TrashResult{}, fmt.Errorf("CommandService.TrashMediaItem: %w", ErrTrashNoFile)
	}
	lib, err := s.scan.libQueries.GetByID(ctx, item.LibraryID)
	if err != nil {
		return TrashResult{}, fmt.Errorf("CommandService.TrashMediaItem: library: %w", err)
	}
	root := libraryRootOf(lib.Paths, item.File.Path)
	if root == "" {
		return TrashResult{}, fmt.Errorf("CommandService.TrashMediaItem: %w", ErrTrashOutsideLibrary)
	}
	from := item.File.Path
	destDir, dest, err := trashDestination(root, from)
	if err != nil {
		return TrashResult{}, fmt.Errorf("CommandService.TrashMediaItem: %w", err)
	}
	// The item's removal must not depend on the caller staying.
	ctx = context.WithoutCancel(ctx)
	err = rename(from, dest)
	switch {
	case err == nil:
		moveCompanions(from, destDir)
		if derr := s.finishTrash(ctx, id, from, dest); derr != nil {
			return TrashResult{TrashedPath: dest}, fmt.Errorf("CommandService.TrashMediaItem: %w", derr)
		}
		return TrashResult{TrashedPath: dest}, nil
	case errors.Is(err, syscall.EXDEV):
		if prev, loaded := s.trashPending.LoadOrStore(id, dest); loaded {
			return TrashResult{TrashedPath: prev.(string), Pending: true}, nil
		}
		s.enqueueTrash(trashJob{id: id, title: item.Title, from: from, dest: dest, destDir: destDir})
		slog.InfoContext(ctx, "library: move to the trash queued, the file is copied to another disk",
			slog.String("mediaItemID", id), slog.String("from", from), slog.String("to", dest))
		return TrashResult{TrashedPath: dest, Pending: true}, nil
	default:
		return TrashResult{}, fmt.Errorf("CommandService.TrashMediaItem: move to trash: %w", err)
	}
}

// finishTrash removes the item once its file is in the trash.
func (s *CommandService) finishTrash(ctx context.Context, id, from, dest string) error {
	if err := s.DeleteMediaItem(ctx, id); err != nil {
		// The file is in the trash and the row still points at its old place:
		// the next scan sees it missing and offers to remove it. Say so.
		slog.ErrorContext(ctx, "library: file moved to the trash but the item could not be removed",
			slog.String("mediaItemID", id), slog.String("trashed", dest), slog.Any("err", err))
		return err
	}
	slog.InfoContext(ctx, "library: media file moved to the trash",
		slog.String("mediaItemID", id), slog.String("from", from), slog.String("to", dest))
	return nil
}

// enqueueTrash hands a cross-disk move to the worker, started on first use.
// One copy at a time: several would only make the disks seek between them.
func (s *CommandService) enqueueTrash(job trashJob) {
	s.trashOnce.Do(func() {
		s.trashQueue = make(chan trashJob, 256)
		go s.runTrashWorker()
	})
	s.trashQueue <- job
}

func (s *CommandService) runTrashWorker() {
	ctx := s.trashCtx
	if ctx == nil {
		ctx = context.Background()
	}
	for job := range s.trashQueue {
		if s.trashHold != nil {
			<-s.trashHold
		}
		s.runTrashJob(ctx, job)
	}
}

func (s *CommandService) runTrashJob(ctx context.Context, job trashJob) {
	defer s.trashPending.Delete(job.id)
	evt := library.MediaTrashFinished{MediaItemID: shared.MediaItemID(job.id), Title: job.title, TrashedPath: job.dest}
	err := copyThenRemove(ctx, job.from, job.dest)
	if err == nil {
		moveCompanions(job.from, job.destDir)
		err = s.finishTrash(ctx, job.id, job.from, job.dest)
	} else {
		slog.WarnContext(ctx, "library: move to the trash failed, the file stays",
			slog.String("mediaItemID", job.id), slog.String("file", job.from), slog.Any("err", err))
	}
	if err != nil {
		evt.Err = err.Error()
	}
	evt.OccurredAt = time.Now()
	if s.events != nil {
		if perr := s.events.Publish(context.WithoutCancel(ctx), evt); perr != nil {
			slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", perr))
		}
	}
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

// trashDestination makes the trash folder for path and picks its name
// there: <root>/.evey-trash/<same relative path>, a timestamp added when the
// name is taken (the same title trashed twice), never an overwrite.
func trashDestination(root, path string) (string, string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", "", ErrTrashOutsideLibrary
	}
	destDir := filepath.Join(root, TrashDirName, filepath.Dir(rel))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create trash folder: %w", err)
	}
	return destDir, freeName(filepath.Join(destDir, filepath.Base(path))), nil
}

// moveToTrash moves the file and its companions into the trash in one go,
// copying across disks if it must. TrashMediaItem splits the same steps to
// answer before a long copy; this whole form serves the tests and tools.
func moveToTrash(root, path string) (string, error) {
	destDir, dest, err := trashDestination(root, path)
	if err != nil {
		return "", err
	}
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
	return copyThenRemove(context.Background(), from, to)
}

// copyThenRemove is the cross-disk half of moveFile. ctx stops it between
// chunks (a server shutting down): the original stays, the partial goes.
func copyThenRemove(ctx context.Context, from, to string) (err error) {
	// O_NOFOLLOW and a regular file only: a copy, unlike a rename, reads what
	// a link points at, and a link planted in a library could otherwise bring
	// a server secret into it.
	src, err := os.OpenFile(from, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // G304 FP: a library media file the trash already resolved under its library root
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("copy across disks: %s is not a regular file", from)
	}
	partial := to + partialSuffix
	// A partial left by a copy a shutdown cut short is ours (only Evey writes
	// in the trash); it would block this copy for good.
	if st, serr := os.Lstat(partial); serr == nil && st.Mode().IsRegular() {
		if rmErr := os.Remove(partial); rmErr != nil { //nolint:gosec // G703 FP: a .evey-partial file inside the trash folder, written by Evey alone
			return fmt.Errorf("clear a partial copy left behind: %w", rmErr)
		}
	}
	dst, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()) //nolint:gosec // G304 FP: next to a destination under the library root
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = dst.Close()
			if rmErr := os.Remove(partial); rmErr != nil { //nolint:gosec // G703 FP: the partial copy this function created next to a destination under the library root
				slog.Warn("library: partial copy left behind", slog.String("file", partial), slog.Any("err", rmErr))
			}
		}
	}()
	n, err := io.CopyBuffer(dst, ctxReader{ctx: ctx, r: src}, make([]byte, 4<<20))
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
	if tErr := os.Chtimes(partial, info.ModTime(), info.ModTime()); tErr != nil {
		slog.Debug("library: copy keeps its own date", slog.String("file", partial), slog.Any("err", tErr))
	}
	if err = rename(partial, to); err != nil {
		return err
	}
	if rmErr := os.Remove(from); rmErr != nil { //nolint:gosec // G703 FP: the media file the trash resolved under its library root, removed only once its copy is whole
		// The copy is whole and in place; the original could not go. Leave
		// both rather than lose either, and say so.
		slog.Warn("library: moved by copy, the original could not be removed", slog.String("file", from), slog.Any("err", rmErr))
	}
	return nil
}

// ctxReader stops a copy when ctx ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
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
