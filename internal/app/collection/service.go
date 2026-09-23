package collection

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// SmartQuerier evaluates collection rules against media_items.
type SmartQuerier interface {
	QueryByRules(ctx context.Context, rules []library.CollectionRule, sortBy, sortOrder string, limit, offset int) ([]*library.MediaItem, error)
	// CountByRules returns how many media items match the rules - for accurate
	// pagination totals without materializing every hydrated row.
	CountByRules(ctx context.Context, rules []library.CollectionRule) (int, error)
}

// MediaItemGetter is a narrow read interface for media item lookups.
type MediaItemGetter interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error)
}
