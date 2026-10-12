package mkv

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Service answers the player's chapter lookup. The rest of the MKV toolbox
// (extract, mux, demux, add/remove track) had no client and took free disk
// paths; it is not exposed any more (2026-09-26).
type Service struct {
	reader mkv.ContainerReader
}

func NewService(reader mkv.ContainerReader) *Service {
	return &Service{reader: reader}
}

func (s *Service) ListChapters(ctx context.Context, path string) ([]mkv.Chapter, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mkv.ListChapters")
	defer span.End()
	return s.reader.ReadChapters(ctx, path)
}
