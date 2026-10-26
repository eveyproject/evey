package playlist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type CommandService struct {
	playlists     playlist.PlaylistRepository
	items         playlist.PlaylistItemRepository
	mediaChecker  MediaItemExistsChecker
	musicChecker  MusicTrackChecker
	libraryAccess PlaylistLibraryAccessChecker
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

func (s *CommandService) WithLibraryAccessChecker(checker PlaylistLibraryAccessChecker) *CommandService {
	s.libraryAccess = checker
	return s
}

func (s *CommandService) Create(ctx context.Context, pl *playlist.Playlist) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.Create")
	defer span.End()
	ptype := pl.Type
	if ptype == "" {
		ptype = playlist.Manual
	}
	created, err := playlist.NewPlaylist(pl.Name, string(pl.ProfileID), ptype)
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

func (s *CommandService) Update(ctx context.Context, pl *playlist.Playlist, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.Update")
	defer span.End()
	if pl.Name == "" {
		return errors.New("invalid playlist: playlist name is required")
	}
	existing, err := s.playlists.GetByID(ctx, pl.ID)
	if err != nil {
		return fmt.Errorf("CommandService.Update: %w", err)
	}
	if !existing.IsOwnedBy(profileID) {
		return ErrNotOwner
	}
	pl.ProfileID = existing.ProfileID
	pl.Type = existing.Type
	pl.CreatedAt = existing.CreatedAt
	pl.UpdatedAt = time.Now()
	return s.playlists.Update(ctx, pl)
}

// id and profileID were two adjacent bare strings deciding ownership - the
// same shape fixed in fcdef433d for the library-access check. Typed the same
// way: profileID is shared.ProfileID so a future accidental argument swap
// with a resource id fails to compile instead of silently deleting/exposing
// the wrong profile's data.
func (s *CommandService) Delete(ctx context.Context, id string, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.Delete")
	defer span.End()
	existing, err := s.playlists.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("CommandService.Delete: %w", err)
	}
	if !existing.IsOwnedBy(profileID) {
		return ErrNotOwner
	}
	return s.playlists.Delete(ctx, id)
}

func (s *CommandService) AddItem(ctx context.Context, playlistID, mediaItemID string, profileID shared.ProfileID, mediaType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.AddItem")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.AddItem: %w", err)
	}
	if !pl.IsOwnedBy(profileID) {
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
			item, checkErr := s.mediaChecker.GetByID(ctx, mediaItemID)
			if errors.Is(checkErr, shared.ErrNotFound) {
				return ErrMediaItemNotFound
			}
			if checkErr != nil {
				return fmt.Errorf("check media item: %w", checkErr)
			}
			// Library-access gate: a profile may only add media from a library it
			// can access. Returning ErrMediaItemNotFound (not a distinct permission
			// error) keeps an inaccessible item indistinguishable from a
			// non-existent one, so a guessed media id can't be probed for existence.
			if s.libraryAccess != nil && item != nil {
				ok, accErr := s.libraryAccess.HasAccessForProfile(ctx, profileID, item.LibraryID)
				if accErr != nil {
					return fmt.Errorf("check library access: %w", accErr)
				}
				if !ok {
					return ErrMediaItemNotFound
				}
			}
		}
	}
	// sort_order AND the item cap are enforced atomically inside the INSERT:
	// the old Go-side count-then-insert let two concurrent adds at cap-1 both
	// pass. The repo's sentinel maps back to this service's public error.
	err = s.items.AddNextOrder(ctx, &playlist.PlaylistItem{
		PlaylistID:  playlistID,
		MediaItemID: shared.MediaItemID(mediaItemID),
		AddedAt:     time.Now(),
		MediaType:   mediaType,
	}, maxItems)
	if errors.Is(err, playlist.ErrMaxItemsReached) {
		return ErrMaxItems
	}
	return err
}

func (s *CommandService) RemoveItem(ctx context.Context, playlistID, mediaItemID string, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.RemoveItem")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.RemoveItem: %w", err)
	}
	if !pl.IsOwnedBy(profileID) {
		return ErrNotOwner
	}
	if pl.IsSmart() {
		return ErrSmartItems
	}
	return s.items.Remove(ctx, playlistID, mediaItemID)
}

func (s *CommandService) ReorderItems(ctx context.Context, playlistID string, profileID shared.ProfileID, mediaItemIDs []string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.ReorderItems")
	defer span.End()
	pl, err := s.playlists.GetByID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.ReorderItems: %w", err)
	}
	if !pl.IsOwnedBy(profileID) {
		return ErrNotOwner
	}
	// The repo UPDATE matches on membership, so an id the playlist does not
	// hold silently changes nothing - name the strangers instead of letting a
	// client bug look like success.
	existing, err := s.items.GetByPlaylistID(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("CommandService.ReorderItems: load items: %w", err)
	}
	members := make(map[string]struct{}, len(existing))
	for _, it := range existing {
		members[string(it.MediaItemID)] = struct{}{}
	}
	var foreign []string
	seen := make(map[string]struct{}, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if _, ok := members[id]; ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		foreign = append(foreign, id)
	}
	if len(foreign) > 0 {
		return fmt.Errorf("%w: %s", ErrForeignItems, strings.Join(foreign, ", "))
	}
	return s.items.Reorder(ctx, playlistID, mediaItemIDs)
}

// SubscribeMediaRemovals purges the playlist items orphaned by a media removal
// or by a library deletion, whose cascade removes media without an event each.
func (s *CommandService) SubscribeMediaRemovals(bus shared.EventBus) {
	purge := func(ctx context.Context, ev shared.Event) {
		n, err := s.items.PurgeOrphans(ctx)
		if err != nil {
			slog.WarnContext(ctx, "playlist: purge orphaned items failed", slog.String("event", ev.EventName()), slog.Any("err", err))
			return
		}
		if n > 0 {
			slog.InfoContext(ctx, "playlist: purged orphaned items", slog.Int64("count", n))
		}
	}
	bus.Subscribe(library.MediaItemRemoved{}.EventName(), purge)
	bus.Subscribe(library.LibraryDeleted{}.EventName(), purge)
}
