package filesystem

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/pkg/filehash"
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
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
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

func (s *MusicScanner) OpenFile(path string) (music.ReadSeekCloser, error) {
	return os.Open(path)
}

func (s *MusicScanner) ComputeHash(path string) (string, error) {
	return filehash.Compute(path)
}
