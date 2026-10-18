package filesystem

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/pkg/filehash"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	ctx, span := tracing.StartSpan(ctx, "filesystem.photoscanner.WalkImageFiles")
	defer span.End()

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// The ROOT failing means the walk saw NOTHING - a dead mount must
			// surface as an error, not read as an empty library. A per-file
			// error further down stays a skip. Same contract as the music
			// scanner.
			if path == dir {
				return fmt.Errorf("photo scan: library root %s: %w", dir, walkErr)
			}
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

// maxExifReadBytes caps how much of a photo is pulled into memory for EXIF
// extraction. EXIF (the JPEG APP1 segment) sits at the start of the file, so
// the header is enough; reading whole files would risk OOM during a scan
// (each of NumCPU workers allocating a full multi-GB file at once).
const maxExifReadBytes = 2 << 20 // 2 MiB

// ReadFile returns the leading bytes of path for metadata (EXIF) extraction,
// bounded to maxExifReadBytes. It is not a whole-file read.
// Exists reports whether the file is still on disk - the prune path's
// last-resort check before deleting a row.
func (s *PhotoScanner) Exists(path string) bool {
	return notProvenGone(path)
}

func (s *PhotoScanner) ReadFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxExifReadBytes))
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
