package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	"github.com/eveyproject/evey/internal/domain/library"
)

// sourceAccessWalkLimit bounds the entries visited to find one media file
// under a library path.
const sourceAccessWalkLimit = 2000

var errFoundMediaFile = errors.New("found")

// sourceAccessExt are the extensions worth sniffing; any other file is skipped
// without being opened.
var sourceAccessExt = map[string]bool{".mkv": true, ".webm": true, ".mp4": true, ".m4v": true, ".mov": true}

// logLibrarySourceAccess logs, for each library path, what mkvgo's SourceAuto
// decides for a media file under it: local (read-ahead asked for) or network.
// A file rather than the root: on a mergerfs pool a file's device can differ
// from the root's. Async, a dead network mount must not hold the boot.
func logLibrarySourceAccess(ctx context.Context, libs library.LibraryQueryRepo, log *slog.Logger) {
	go func() {
		list, err := libs.List(ctx, library.ListLibrariesFilter{})
		if err != nil {
			log.Warn("mkvgo source access: libraries not listed", slog.Any("err", err))
			return
		}
		for _, lib := range list {
			for _, root := range lib.Paths {
				file := firstMediaFile(root)
				if file == "" {
					log.Info("mkvgo source access: no media file under the library path", slog.String("library", lib.Name), slog.String("path", root))
					continue
				}
				a, err := mkvgoadapter.SourceAccessOf(file)
				if err != nil {
					log.Warn("mkvgo source access: file not opened", slog.String("library", lib.Name), slog.String("path", root), slog.Any("err", err))
					continue
				}
				log.Info("mkvgo source access (auto)", slog.String("library", lib.Name), slog.String("path", root),
					slog.String("access", a.Access()), slog.String("fs_type", a.FSType), slog.String("file", file))
			}
		}
	}()
}

// firstMediaFile is the first Matroska or MP4 video file under root, "" when none
// within sourceAccessWalkLimit entries.
func firstMediaFile(root string) string {
	var found string
	seen := 0
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if seen++; seen > sourceAccessWalkLimit {
			return filepath.SkipAll
		}
		if err != nil {
			// Unreadable: skip it (a file's error skips the rest of its
			// directory, enough when one media file is all that is wanted).
			return filepath.SkipDir
		}
		if d.IsDir() || !sourceAccessExt[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		if mkvgoadapter.IsEBMLContainer(p) || mkvgoadapter.IsMP4Container(p) {
			found = p
			return errFoundMediaFile
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errFoundMediaFile) {
		return ""
	}
	return found
}
