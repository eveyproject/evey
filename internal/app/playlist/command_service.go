package playlist

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type CommandService struct {
	playlists    playlist.PlaylistRepository
	items        playlist.PlaylistItemRepository
	mediaChecker MediaItemExistsChecker
	musicChecker MusicTrackChecker
}

func NewCommandService(
	playlists playlist.PlaylistRepository,
	items playlist.PlaylistItemRepository,
) *CommandService {
	return &CommandService{playlists: playlists, items: items}
}

func (s *CommandService) WithMediaChecker(checker MediaItemExistsChecker) *CommandService {
	s.mediaChecker = checker
	return s
}

func (s *CommandService) WithMusicTrackChecker(checker MusicTrackChecker) *CommandService {
	s.musicChecker = checker
	return s
}

func (s *CommandService) Create(ctx context.Context, pl *playlist.Playlist) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.Create")
	defer span.End()
	ptype := pl.Type
	if ptype == "" {
		ptype = playlist.Manual
	}
	created, err := playlist.NewPlaylist(pl.Name, string(pl.UserID), ptype)
	if err != nil {
		return fmt.Errorf("invalid playlist: %w", err)
	}
	created.Description = pl.Description
	created.Rules = pl.Rules
	created.SortBy = pl.SortBy
	created.SortOrder = pl.SortOrder
	if created.SortBy == "" {
		created.SortBy = "sort_order"
	}
	if created.SortOrder == "" {
		created.SortOrder = "asc"
	}
	// Mutate caller's struct so callers keep access to generated ID/timestamps.
	*pl = *created
	return s.playlists.Create(ctx, pl)
}

func (s *CommandService) Update(ctx context.Context, pl *playlist.Playlist, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.Update")
	defer span.End()
	if pl.Name == "" {
		return errors.New("invalid playlist: playlist name is required")
	}
	existing, err := s.playlists.GetByID(ctx, pl.ID)
	if err != nil {
		return fmt.Errorf("CommandService.Update: %w", err)
	}
	if !existing.IsOwner(userID) {
		return ErrNotOwner
	}
	pl.UserID = existing.UserID
	pl.Type = existing.Type
	pl.CreatedAt = existing.CreatedAt
	pl.UpdatedAt = time.Now()
	return s.playlists.Update(ctx, pl)
}

func (s *CommandService) Delete(ctx context.Context, id, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.Delete")
	defer span.End()
	existing, err := s.playlists.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("CommandService.Delete: %w", err)
	}
	if !existing.IsOwner(userID) {
		return ErrNotOwner
	}
	return s.playlists.Delete(ctx, id)
}

func (s *CommandService) AddItem(ctx context.Context, playlistID, mediaItemID, userID, mediaType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.AddItem")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.AddItem: %w", err)
	}
	if !pl.IsOwner(userID) {
		return ErrNotOwner
	}
	if pl.IsSmart() {
		return ErrSmartItems
	}
	if mediaType == "" {
		mediaType = playlist.MediaTypeVideo
	}
	switch mediaType {
	case playlist.MediaTypeMusic:
		if s.musicChecker != nil {
			ok, checkErr := s.musicChecker.Exists(ctx, mediaItemID)
			if errors.Is(checkErr, shared.ErrNotFound) || (checkErr == nil && !ok) {
				return ErrMediaItemNotFound
			}
			if checkErr != nil {
				return fmt.Errorf("check music track: %w", checkErr)
			}
		}
	default:
		if s.mediaChecker != nil {
			_, checkErr := s.mediaChecker.GetByID(ctx, mediaItemID)
			if errors.Is(checkErr, shared.ErrNotFound) {
				return ErrMediaItemNotFound
			}
			if checkErr != nil {
				return fmt.Errorf("check media item: %w", checkErr)
			}
		}
	}
	existing, err := s.items.GetByPlaylistID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("get items: %w", err)
	}
	if len(existing) >= maxItems {
		return ErrMaxItems
	}
	nextOrder := 0
	for _, it := range existing {
		if it.SortOrder >= nextOrder {
			nextOrder = it.SortOrder + 1
		}
	}
	return s.items.Add(ctx, &playlist.PlaylistItem{
		PlaylistID:  playlistID,
		MediaItemID: shared.MediaItemID(mediaItemID),
		SortOrder:   nextOrder,
		AddedAt:     time.Now(),
		MediaType:   mediaType,
	})
}

func (s *CommandService) RemoveItem(ctx context.Context, playlistID, mediaItemID, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.RemoveItem")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.RemoveItem: %w", err)
	}
	if !pl.IsOwner(userID) {
		return ErrNotOwner
	}
	if pl.IsSmart() {
		return ErrSmartItems
	}
	return s.items.Remove(ctx, playlistID, mediaItemID)
}

func (s *CommandService) ReorderItems(ctx context.Context, playlistID, userID string, mediaItemIDs []string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.ReorderItems")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.ReorderItems: %w", err)
	}
	if !pl.IsOwner(userID) {
		return ErrNotOwner
	}
	return s.items.Reorder(ctx, playlistID, mediaItemIDs)
}
