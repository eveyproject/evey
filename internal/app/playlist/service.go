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

// MusicTrackChecker verifies a music track exists.
// Implementations return (nil, nil) OR (nil, shared.ErrNotFound) when absent.
type MusicTrackChecker interface {
	Exists(ctx context.Context, id string) (bool, error)
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
	mediaReader MediaItemReader,
) *Service {
	cmd := NewCommandService(playlists, items)
	q := NewQueryService(playlists, items, smartQuerier, mediaReader)
	return &Service{cmd: cmd, query: q}
}

func (s *Service) CommandService() *CommandService { return s.cmd }
func (s *Service) QueryService() *QueryService     { return s.query }

func (s *Service) WithMediaChecker(checker MediaItemExistsChecker) *Service {
	s.cmd.WithMediaChecker(checker)
	return s
}

func (s *Service) WithMusicTrackChecker(checker MusicTrackChecker) *Service {
	s.cmd.WithMusicTrackChecker(checker)
	return s
}

func (s *Service) Create(ctx context.Context, pl *playlist.Playlist) error {
	return s.cmd.Create(ctx, pl)
}

func (s *Service) Update(ctx context.Context, pl *playlist.Playlist, profileID string) error {
	return s.cmd.Update(ctx, pl, profileID)
}

func (s *Service) Delete(ctx context.Context, id, profileID string) error {
	return s.cmd.Delete(ctx, id, profileID)
}

func (s *Service) AddItem(ctx context.Context, playlistID, mediaItemID, profileID string) error {
	return s.cmd.AddItem(ctx, playlistID, mediaItemID, profileID, "")
}

func (s *Service) AddItemWithType(ctx context.Context, playlistID, mediaItemID, profileID, mediaType string) error {
	return s.cmd.AddItem(ctx, playlistID, mediaItemID, profileID, mediaType)
}

func (s *Service) RemoveItem(ctx context.Context, playlistID, mediaItemID, profileID string) error {
	return s.cmd.RemoveItem(ctx, playlistID, mediaItemID, profileID)
}

func (s *Service) ReorderItems(ctx context.Context, playlistID, profileID string, mediaItemIDs []string) error {
	return s.cmd.ReorderItems(ctx, playlistID, profileID, mediaItemIDs)
}

func (s *Service) GetByID(ctx context.Context, id, profileID string) (*playlist.Playlist, error) {
	return s.query.GetByID(ctx, id, profileID)
}

func (s *Service) List(ctx context.Context, profileID string) ([]*playlist.Playlist, error) {
	return s.query.List(ctx, profileID)
}

func (s *Service) GetItems(ctx context.Context, playlistID, profileID string) ([]*library.MediaItem, []*playlist.PlaylistItem, error) {
	return s.query.GetItems(ctx, playlistID, profileID)
}
