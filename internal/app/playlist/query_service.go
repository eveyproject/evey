package playlist

import (
	"context"
	"fmt"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// MediaItemReader hydrates a list of media item IDs into full MediaItem
// aggregates. Narrow port by design - the playlist app layer only needs
// batch reads, nothing else.
type MediaItemReader interface {
	GetByIDs(ctx context.Context, ids []shared.MediaItemID) ([]*library.MediaItem, error)
}

type QueryService struct {
	playlists    playlist.PlaylistRepository
	items        playlist.PlaylistItemRepository
	smartQuerier playlist.SmartPlaylistQuerier
	mediaReader  MediaItemReader
}

func NewQueryService(
	playlists playlist.PlaylistRepository,
	items playlist.PlaylistItemRepository,
	smartQuerier playlist.SmartPlaylistQuerier,
	mediaReader MediaItemReader,
) *QueryService {
	return &QueryService{
		playlists:    playlists,
		items:        items,
		smartQuerier: smartQuerier,
		mediaReader:  mediaReader,
	}
}

func (s *QueryService) GetByID(ctx context.Context, id string, profileID shared.ProfileID) (*playlist.Playlist, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.GetByID")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !pl.IsOwnedBy(profileID) {
		return nil, ErrNotOwner
	}
	return pl, nil
}

func (s *QueryService) List(ctx context.Context, profileID shared.ProfileID) ([]*playlist.Playlist, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.List")
	defer span.End()
	return s.playlists.List(ctx, string(profileID))
}

func (s *QueryService) GetItems(ctx context.Context, playlistID string, profileID shared.ProfileID) ([]*library.MediaItem, []*playlist.PlaylistItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.GetItems")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return nil, nil, err
	}
	if !pl.IsOwnedBy(profileID) {
		return nil, nil, ErrNotOwner
	}
	if pl.IsSmart() {
		spec, err := playlist.CompileRules(pl.Rules)
		if err != nil {
			return nil, nil, fmt.Errorf("compile rules: %w", err)
		}
		ids, err := s.smartQuerier.Query(ctx, spec, pl.SortBy, pl.SortOrder, maxItems)
		if err != nil {
			return nil, nil, err
		}
		if len(ids) == 0 {
			return nil, nil, nil
		}
		items, err := s.mediaReader.GetByIDs(ctx, ids)
		if err != nil {
			return nil, nil, fmt.Errorf("hydrate smart playlist: %w", err)
		}
		return preserveOrder(ids, items), nil, nil
	}
	plItems, err := s.items.GetByPlaylistID(ctx, playlistID)
	if err != nil {
		return nil, nil, err
	}
	return nil, plItems, nil
}

// preserveOrder reorders items to match ids. GetByIDs implementations may
// reorder results (postgres does, the fake does not); smart playlists must
// honor the query's sort order so reorder deterministically here.
func preserveOrder(ids []shared.MediaItemID, items []*library.MediaItem) []*library.MediaItem {
	idx := make(map[shared.MediaItemID]*library.MediaItem, len(items))
	for _, it := range items {
		idx[it.ID] = it
	}
	out := make([]*library.MediaItem, 0, len(ids))
	for _, id := range ids {
		if it, ok := idx[id]; ok {
			out = append(out, it)
		}
	}
	return out
}
