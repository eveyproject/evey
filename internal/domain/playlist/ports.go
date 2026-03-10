package playlist

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type PlaylistRepository interface {
	Create(ctx context.Context, pl *Playlist) error
	GetByID(ctx context.Context, id string) (*Playlist, error)
	List(ctx context.Context, userID string) ([]*Playlist, error)
	Update(ctx context.Context, pl *Playlist) error
	Delete(ctx context.Context, id string) error
}

type PlaylistItemRepository interface {
	Add(ctx context.Context, item *PlaylistItem) error
	Remove(ctx context.Context, playlistID, mediaItemID string) error
	GetByPlaylistID(ctx context.Context, playlistID string) ([]*PlaylistItem, error)
	Reorder(ctx context.Context, playlistID string, mediaItemIDs []string) error
}

type SmartPlaylistQuerier interface {
	QueryByRules(ctx context.Context, rules []Rule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error)
}
