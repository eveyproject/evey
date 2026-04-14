package playlist

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playlist"
)

type QueryService struct {
	playlists    playlist.PlaylistRepository
	items        playlist.PlaylistItemRepository
	smartQuerier playlist.SmartPlaylistQuerier
}

func NewQueryService(
	playlists playlist.PlaylistRepository,
	items playlist.PlaylistItemRepository,
	smartQuerier playlist.SmartPlaylistQuerier,
) *QueryService {
	return &QueryService{playlists: playlists, items: items, smartQuerier: smartQuerier}
}

func (s *QueryService) GetByID(ctx context.Context, id, userID string) (*playlist.Playlist, error) {
	pl, err := s.playlists.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !pl.IsOwner(userID) {
		return nil, ErrNotOwner
	}
	return pl, nil
}

func (s *QueryService) List(ctx context.Context, userID string) ([]*playlist.Playlist, error) {
	return s.playlists.List(ctx, userID)
}

func (s *QueryService) GetItems(ctx context.Context, playlistID, userID string) ([]*library.MediaItem, []*playlist.PlaylistItem, error) {
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return nil, nil, err
	}
	if !pl.IsOwner(userID) {
		return nil, nil, ErrNotOwner
	}
	if pl.IsSmart() {
		var items []*library.MediaItem
		items, err = s.smartQuerier.QueryByRules(ctx, pl.Rules, pl.SortBy, pl.SortOrder, maxItems)
		return items, nil, err
	}
	plItems, err := s.items.GetByPlaylistID(ctx, playlistID)
	if err != nil {
		return nil, nil, err
	}
	return nil, plItems, nil
}
