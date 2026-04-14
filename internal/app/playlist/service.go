package playlist

import (
	"context"
	"errors"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playlist"
)

const maxItems = 500

var (
	ErrNotOwner          = errors.New("not playlist owner")
	ErrSmartItems        = errors.New("cannot manually add items to smart playlist")
	ErrMaxItems          = errors.New("playlist item limit reached")
	ErrMediaItemNotFound = errors.New("media item not found")
)

type MediaItemExistsChecker interface {
	GetByID(ctx context.Context, id string) (*library.MediaItem, error)
}

// Service wraps CommandService + QueryService for backward compatibility.
type Service struct {
	cmd   *CommandService
	query *QueryService
}

func NewService(
	playlists playlist.PlaylistRepository,
	items playlist.PlaylistItemRepository,
	smartQuerier playlist.SmartPlaylistQuerier,
) *Service {
	cmd := NewCommandService(playlists, items)
	q := NewQueryService(playlists, items, smartQuerier)
	return &Service{cmd: cmd, query: q}
}

func (s *Service) CommandService() *CommandService { return s.cmd }
func (s *Service) QueryService() *QueryService     { return s.query }

func (s *Service) WithMediaChecker(checker MediaItemExistsChecker) *Service {
	s.cmd.WithMediaChecker(checker)
	return s
}

func (s *Service) Create(ctx context.Context, pl *playlist.Playlist) error {
	return s.cmd.Create(ctx, pl)
}

func (s *Service) Update(ctx context.Context, pl *playlist.Playlist, userID string) error {
	return s.cmd.Update(ctx, pl, userID)
}

func (s *Service) Delete(ctx context.Context, id, userID string) error {
	return s.cmd.Delete(ctx, id, userID)
}

func (s *Service) AddItem(ctx context.Context, playlistID, mediaItemID, userID string) error {
	return s.cmd.AddItem(ctx, playlistID, mediaItemID, userID)
}

func (s *Service) RemoveItem(ctx context.Context, playlistID, mediaItemID, userID string) error {
	return s.cmd.RemoveItem(ctx, playlistID, mediaItemID, userID)
}

func (s *Service) ReorderItems(ctx context.Context, playlistID, userID string, mediaItemIDs []string) error {
	return s.cmd.ReorderItems(ctx, playlistID, userID, mediaItemIDs)
}

func (s *Service) GetByID(ctx context.Context, id, userID string) (*playlist.Playlist, error) {
	return s.query.GetByID(ctx, id, userID)
}

func (s *Service) List(ctx context.Context, userID string) ([]*playlist.Playlist, error) {
	return s.query.List(ctx, userID)
}

func (s *Service) GetItems(ctx context.Context, playlistID, userID string) ([]*library.MediaItem, []*playlist.PlaylistItem, error) {
	return s.query.GetItems(ctx, playlistID, userID)
}
