package filesystem

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/pkg/filehash"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

var audioExts = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".wav":  "audio/wav",
	".aac":  "audio/aac",
}

type MusicScanner struct{}

func NewMusicScanner() *MusicScanner {
	return &MusicScanner{}
}

func (s *MusicScanner) WalkAudioFiles(ctx context.Context, dir string, fn func(music.ScannedAudioFile) error) error {
	ctx, span := tracing.StartSpan(ctx, "filesystem.musicscanner.WalkAudioFiles")
	defer span.End()

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// The ROOT failing means the walk saw NOTHING - a dead mount must
			// surface as an error, not read as an empty library (prod
			// 2026-08-24: /media/musique vanished and the scan reported
			// "nothing new"). A per-file error further down stays a skip.
			if path == dir {
				return fmt.Errorf("music scan: library root %s: %w", dir, walkErr)
			}
			slog.Warn("music scan: walk error", slog.String("path", path), slog.Any("err", walkErr))
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		mimeType, ok := audioExts[ext]
		if !ok {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			slog.Warn("music scan: stat failed", slog.String("path", path), slog.Any("err", statErr))
			return nil
		}
		return fn(music.ScannedAudioFile{
			Path:     path,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			MimeType: mimeType,
		})
	})
}

// Exists reports whether the file is still on disk - the prune path's
// last-resort check before deleting a row.
func (s *MusicScanner) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *MusicScanner) OpenFile(path string) (music.ReadSeekCloser, error) {
	return os.Open(path)
}

func (s *MusicScanner) ComputeHash(path string) (string, error) {
	return filehash.Compute(path)
}
