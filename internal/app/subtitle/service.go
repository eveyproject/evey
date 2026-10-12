package subtitle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/subtitle"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type Service struct {
	repo     subtitle.Repository
	provider subtitle.Provider
	media    subtitle.MediaItemGetter
}

func NewService(repo subtitle.Repository, provider subtitle.Provider, media subtitle.MediaItemGetter) *Service {
	return &Service{repo: repo, provider: provider, media: media}
}

func (s *Service) Search(ctx context.Context, mediaItemID, language string) ([]subtitle.SearchResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.subtitle.Search")
	defer span.End()

	item, err := s.media.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return nil, fmt.Errorf("get media item: %w", err)
	}
	hash := computeOpenSubtitlesHash(item.FilePath)
	return s.provider.Search(ctx, hash, item.Title, language)
}

func (s *Service) Download(ctx context.Context, mediaItemID string, fileID int, language, format string) (*subtitle.SubtitleDownload, error) {
	ctx, span := tracing.StartSpan(ctx, "app.subtitle.Download")
	defer span.End()

	item, err := s.media.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return nil, fmt.Errorf("get media item: %w", err)
	}
	destDir := filepath.Join(filepath.Dir(item.FilePath), ".subtitles")
	filePath, err := s.provider.Download(ctx, fileID, destDir)
	if err != nil {
		return nil, fmt.Errorf("download subtitle: %w", err)
	}
	if format == "" {
		format = "srt"
	}
	sub, err := subtitle.NewSubtitleDownload(id.New("sub-"), shared.MediaItemID(mediaItemID), language, filePath,
		subtitle.WithSubtitleFormat(format),
		subtitle.WithSubtitleSource("opensubtitles"),
	)
	if err != nil {
		return nil, fmt.Errorf("build subtitle: %w", err)
	}
	if err := s.repo.Save(ctx, sub); err != nil {
		return nil, fmt.Errorf("save subtitle: %w", err)
	}
	return sub, nil
}

func (s *Service) List(ctx context.Context, mediaItemID string) ([]*subtitle.SubtitleDownload, error) {
	ctx, span := tracing.StartSpan(ctx, "app.subtitle.List")
	defer span.End()

	return s.repo.GetByMediaItem(ctx, mediaItemID)
}

func (s *Service) Delete(ctx context.Context, subtitleID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.subtitle.Delete")
	defer span.End()

	return s.repo.Delete(ctx, subtitleID)
}

// computeOpenSubtitlesHash computes the hash used by OpenSubtitles (first+last 64KB).
// Returns empty string on error (caller falls back to title search).
func computeOpenSubtitlesHash(filePath string) string {
	if filePath == "" {
		return ""
	}
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.Size() < 65536*2 {
		return ""
	}

	size := uint64(fi.Size())
	hash := size

	buf := make([]byte, 65536)
	if _, err := f.Read(buf); err != nil {
		return ""
	}
	for i := 0; i < 65536; i += 8 {
		hash += uint64(buf[i]) | uint64(buf[i+1])<<8 | uint64(buf[i+2])<<16 | uint64(buf[i+3])<<24 |
			uint64(buf[i+4])<<32 | uint64(buf[i+5])<<40 | uint64(buf[i+6])<<48 | uint64(buf[i+7])<<56
	}

	if _, err := f.Seek(-65536, 2); err != nil {
		return ""
	}
	if _, err := f.Read(buf); err != nil {
		return ""
	}
	for i := 0; i < 65536; i += 8 {
		hash += uint64(buf[i]) | uint64(buf[i+1])<<8 | uint64(buf[i+2])<<16 | uint64(buf[i+3])<<24 |
			uint64(buf[i+4])<<32 | uint64(buf[i+5])<<40 | uint64(buf[i+6])<<48 | uint64(buf[i+7])<<56
	}

	return fmt.Sprintf("%016x", hash)
}
