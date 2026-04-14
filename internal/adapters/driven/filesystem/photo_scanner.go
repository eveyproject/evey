package filesystem

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/pkg/filehash"
)

var imageExts = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".heic": "image/heic",
	".webp": "image/webp",
	".gif":  "image/gif",
	".tiff": "image/tiff",
	".tif":  "image/tiff",
	".bmp":  "image/bmp",
}

type PhotoScanner struct{}

func NewPhotoScanner() *PhotoScanner {
	return &PhotoScanner{}
}

func (s *PhotoScanner) WalkImageFiles(ctx context.Context, dir string, fn func(photo.ScannedImageFile) error) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			slog.Warn("photo walk error", slog.String("path", path), slog.Any("err", walkErr))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ext := strings.ToLower(filepath.Ext(path))
		mime, ok := imageExts[ext]
		if !ok {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			slog.Warn("photo stat error", slog.String("path", path), slog.Any("err", infoErr))
			return nil
		}
		return fn(photo.ScannedImageFile{
			Path:     path,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			MimeType: mime,
		})
	})
}

func (s *PhotoScanner) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (s *PhotoScanner) ImageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func (s *PhotoScanner) ComputeHash(path string) (string, error) {
	return filehash.Compute(path)
}
