package playlist

import (
	"context"
	"errors"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
)

const maxItems = 500

var (
	ErrNotOwner          = errors.New("not playlist owner")
	ErrSmartItems        = errors.New("cannot manually add items to smart playlist")
	ErrMaxItems          = errors.New("playlist item limit reached")
	ErrMediaItemNotFound = errors.New("media item not found")
	// ErrForeignItems rejects a reorder naming ids the playlist does not hold:
	// the repo UPDATE matches on membership, so a stranger id would silently
	// change nothing and the caller would get a 200 for a half-applied
	// reorder (same contract as collections, decided 2026-08-05).
	ErrForeignItems = errors.New("media_item_ids contains items that are not in this playlist")
)

type MediaItemExistsChecker interface {
	GetByID(ctx context.Context, id string) (*library.MediaItem, error)
}

// MusicTrackChecker verifies a music track exists.
// Implementations return (nil, nil) OR (nil, shared.ErrNotFound) when absent.
type MusicTrackChecker interface {
	Exists(ctx context.Context, id string) (bool, error)
}

// PlaylistLibraryAccessChecker reports whether the given profile may access the
// given library. The adapter resolves the profile to its owning user and
// consults library_access. AddItem uses it so a profile cannot add (and then
// read the metadata of, via GetItems) a media item from a library it has no
// access to, even when it guesses a valid media id.
//
// Les deux identifiants sont TYPÉS, et pas par cosmétique : c'était
// `(profileID, libraryID string)`, deux chaînes voisines dans un contrôle
// d'accès, donc les intervertir compilait sans un mot - vérifié le 08-01, le
// build passait. Un contrôle d'accès inversé autorise silencieusement, il ne
// refuse pas bruyamment : personne ne l'aurait vu.
type PlaylistLibraryAccessChecker interface {
	HasAccessForProfile(ctx context.Context, profileID shared.ProfileID, libraryID shared.LibraryID) (bool, error)
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

func (s *Service) WithLibraryAccessChecker(checker PlaylistLibraryAccessChecker) *Service {
	s.cmd.WithLibraryAccessChecker(checker)
	return s
}

func (s *Service) Create(ctx context.Context, pl *playlist.Playlist) error {
	return s.cmd.Create(ctx, pl)
}

func (s *Service) Update(ctx context.Context, pl *playlist.Playlist, profileID shared.ProfileID) error {
	return s.cmd.Update(ctx, pl, profileID)
}

func (s *Service) Delete(ctx context.Context, id string, profileID shared.ProfileID) error {
	return s.cmd.Delete(ctx, id, profileID)
}

func (s *Service) AddItem(ctx context.Context, playlistID, mediaItemID string, profileID shared.ProfileID) error {
	return s.cmd.AddItem(ctx, playlistID, mediaItemID, profileID, "")
}

func (s *Service) AddItemWithType(ctx context.Context, playlistID, mediaItemID string, profileID shared.ProfileID, mediaType string) error {
	return s.cmd.AddItem(ctx, playlistID, mediaItemID, profileID, mediaType)
}

func (s *Service) RemoveItem(ctx context.Context, playlistID, mediaItemID string, profileID shared.ProfileID) error {
	return s.cmd.RemoveItem(ctx, playlistID, mediaItemID, profileID)
}

func (s *Service) ReorderItems(ctx context.Context, playlistID string, profileID shared.ProfileID, mediaItemIDs []string) error {
	return s.cmd.ReorderItems(ctx, playlistID, profileID, mediaItemIDs)
}

func (s *Service) GetByID(ctx context.Context, id string, profileID shared.ProfileID) (*playlist.Playlist, error) {
	return s.query.GetByID(ctx, id, profileID)
}

func (s *Service) List(ctx context.Context, profileID shared.ProfileID) ([]*playlist.Playlist, error) {
	return s.query.List(ctx, profileID)
}

func (s *Service) GetItems(ctx context.Context, playlistID string, profileID shared.ProfileID) ([]*library.MediaItem, []*playlist.PlaylistItem, error) {
	return s.query.GetItems(ctx, playlistID, profileID)
}
