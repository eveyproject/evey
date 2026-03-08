package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
)

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	".ts": true, ".m2ts": true, ".mpg": true, ".mpeg": true,
	".3gp": true, ".ogv": true, ".vob": true,
}

type Scanner struct{}

func NewScanner() *Scanner {
	return &Scanner{}
}

func (s *Scanner) Scan(ctx context.Context, root string) ([]library.ScannedFile, error) {
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

		info, err := d.Info()
		if err != nil {
			return nil
		}
		parsed := (&FilenameParser{}).Parse(path)
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
