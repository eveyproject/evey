package playlist

import (
	"context"
)

type PlaylistRepository interface {
	Create(ctx context.Context, pl *Playlist) error
	GetByID(ctx context.Context, id string) (*Playlist, error)
	List(ctx context.Context, profileID string) ([]*Playlist, error)
	Update(ctx context.Context, pl *Playlist) error
	Delete(ctx context.Context, id string) error
}

type PlaylistItemRepository interface {
	Add(ctx context.Context, item *PlaylistItem) error
	// AddNextOrder appends item with its sort_order computed atomically in the
	// INSERT (MAX(sort_order)+1), so two concurrent adds can't read the same
	// order in Go and write colliding values. item.SortOrder is ignored.
	AddNextOrder(ctx context.Context, item *PlaylistItem) error
	Remove(ctx context.Context, playlistID, mediaItemID string) error
	GetByPlaylistID(ctx context.Context, playlistID string) ([]*PlaylistItem, error)
	// GetByPlaylistIDs is the batch form of GetByPlaylistID - one query for
	// every playlist instead of one query per playlist (N+1 in the playlist
	// generator's de-duplication scan). Playlists with no items are simply
	// absent from the returned map.
	GetByPlaylistIDs(ctx context.Context, playlistIDs []string) (map[string][]*PlaylistItem, error)
	Reorder(ctx context.Context, playlistID string, mediaItemIDs []string) error
}
