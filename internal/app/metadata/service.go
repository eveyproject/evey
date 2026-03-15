package metadata

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/metadata"
)

// ItemEnricher persists metadata results (artwork, people) for a media item.
type ItemEnricher interface {
	EnrichItem(ctx context.Context, item *library.MediaItem, result *metadata.MetadataResult) error
}
