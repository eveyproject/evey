package playlist

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
)

type CommandService struct {
	playlists    playlist.PlaylistRepository
	items        playlist.PlaylistItemRepository
	mediaChecker MediaItemExistsChecker
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

func (s *CommandService) Create(ctx context.Context, pl *playlist.Playlist) error {
	if pl.Type == "" {
		pl.Type = playlist.Manual
	}
	if err := pl.Validate(); err != nil {
		return fmt.Errorf("invalid playlist: %w", err)
	}
	now := time.Now()
	pl.ID = id.New("pl-")
	pl.CreatedAt = now
	pl.UpdatedAt = now
	if pl.SortBy == "" {
		pl.SortBy = "sort_order"
	}
	if pl.SortOrder == "" {
		pl.SortOrder = "asc"
	}
	return s.playlists.Create(ctx, pl)
}

func (s *CommandService) Update(ctx context.Context, pl *playlist.Playlist, userID string) error {
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
	existing, err := s.playlists.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("CommandService.Delete: %w", err)
	}
	if !existing.IsOwner(userID) {
		return ErrNotOwner
	}
	return s.playlists.Delete(ctx, id)
}

func (s *CommandService) AddItem(ctx context.Context, playlistID, mediaItemID, userID string) error {
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
	if s.mediaChecker != nil {
		_, checkErr := s.mediaChecker.GetByID(ctx, mediaItemID)
		if errors.Is(checkErr, shared.ErrNotFound) {
			return ErrMediaItemNotFound
		}
		if checkErr != nil {
			return fmt.Errorf("check media item: %w", checkErr)
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
	})
}

func (s *CommandService) RemoveItem(ctx context.Context, playlistID, mediaItemID, userID string) error {
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
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.ReorderItems: %w", err)
	}
	if !pl.IsOwner(userID) {
		return ErrNotOwner
	}
	return s.items.Reorder(ctx, playlistID, mediaItemIDs)
}
