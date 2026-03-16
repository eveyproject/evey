package playlist

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/playlist"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

const maxItems = 500

var (
	ErrNotOwner        = errors.New("not playlist owner")
	ErrSmartItems      = errors.New("cannot manually add items to smart playlist")
	ErrMaxItems        = errors.New("playlist item limit reached")
	ErrMediaItemNotFound = errors.New("media item not found")
)

type MediaItemExistsChecker interface {
	GetByID(ctx context.Context, id string) (*library.MediaItem, error)
}

type Service struct {
	playlists    playlist.PlaylistRepository
	items        playlist.PlaylistItemRepository
	smartQuerier playlist.SmartPlaylistQuerier
	mediaChecker MediaItemExistsChecker
}

func NewService(
	playlists playlist.PlaylistRepository,
	items playlist.PlaylistItemRepository,
	smartQuerier playlist.SmartPlaylistQuerier,
) *Service {
	return &Service{
		playlists:    playlists,
		items:        items,
		smartQuerier: smartQuerier,
	}
}

func (s *Service) WithMediaChecker(checker MediaItemExistsChecker) *Service {
	s.mediaChecker = checker
	return s
}

func (s *Service) Create(ctx context.Context, pl *playlist.Playlist) error {
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

func (s *Service) GetByID(ctx context.Context, id, userID string) (*playlist.Playlist, error) {
	pl, err := s.playlists.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !pl.IsOwner(userID) {
		return nil, ErrNotOwner
	}
	return pl, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]*playlist.Playlist, error) {
	return s.playlists.List(ctx, userID)
}

func (s *Service) Update(ctx context.Context, pl *playlist.Playlist, userID string) error {
	if pl.Name == "" {
		return fmt.Errorf("invalid playlist: playlist name is required")
	}
	existing, err := s.playlists.GetByID(ctx, pl.ID)
	if err != nil {
		return err
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

func (s *Service) Delete(ctx context.Context, id, userID string) error {
	existing, err := s.playlists.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !existing.IsOwner(userID) {
		return ErrNotOwner
	}
	return s.playlists.Delete(ctx, id)
}

func (s *Service) AddItem(ctx context.Context, playlistID, mediaItemID, userID string) error {
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return err
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
		MediaItemID: mediaItemID,
		SortOrder:   nextOrder,
		AddedAt:     time.Now(),
	})
}

func (s *Service) RemoveItem(ctx context.Context, playlistID, mediaItemID, userID string) error {
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return err
	}
	if !pl.IsOwner(userID) {
		return ErrNotOwner
	}
	if pl.IsSmart() {
		return ErrSmartItems
	}
	return s.items.Remove(ctx, playlistID, mediaItemID)
}

func (s *Service) ReorderItems(ctx context.Context, playlistID, userID string, mediaItemIDs []string) error {
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return err
	}
	if !pl.IsOwner(userID) {
		return ErrNotOwner
	}
	return s.items.Reorder(ctx, playlistID, mediaItemIDs)
}

func (s *Service) GetItems(ctx context.Context, playlistID, userID string) ([]*library.MediaItem, []*playlist.PlaylistItem, error) {
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
