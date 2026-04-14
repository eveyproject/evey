package playlist

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
)

type SmartPlaylistQuerier interface {
	QueryByRules(ctx context.Context, rules []Rule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error)
}
