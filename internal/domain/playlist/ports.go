package playlist

import (
	"context"
	"errors"
)

// ErrDuplicateName marks a Create/Update refused by the unique
// (profile_id, name) constraint - one profile, one playlist per name.
var ErrDuplicateName = errors.New("playlist name already used by this profile")

// ErrMaxItemsReached marks an AddNextOrder refused by the item cap enforced
// INSIDE the insert statement (the Go-side pre-check alone was racy: two
// concurrent adds at cap-1 both passed it).
var ErrMaxItemsReached = errors.New("playlist item cap reached")

// ErrReorderMembershipDrift reports that a transactional reorder matched
// fewer rows than requested ids: an item left the playlist between the
// caller's membership pre-check and the UPDATE (TOCTOU), or a requested id
// never was a member. The write is rolled back rather than half-applied.
var ErrReorderMembershipDrift = errors.New("playlist reorder: membership changed during reorder")

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
	// maxItems caps the playlist size inside the same statement; a full
	// playlist returns ErrMaxItemsReached, an already-present item stays a
	// silent no-op. maxItems <= 0 means uncapped.
	AddNextOrder(ctx context.Context, item *PlaylistItem, maxItems int) error
	Remove(ctx context.Context, playlistID, mediaItemID string) error
	GetByPlaylistID(ctx context.Context, playlistID string) ([]*PlaylistItem, error)
	// GetByPlaylistIDs is the batch form of GetByPlaylistID - one query for
	// every playlist instead of one query per playlist (N+1 in the playlist
	// generator's de-duplication scan). Playlists with no items are simply
	// absent from the returned map.
	GetByPlaylistIDs(ctx context.Context, playlistIDs []string) (map[string][]*PlaylistItem, error)
	Reorder(ctx context.Context, playlistID string, mediaItemIDs []string) error
	// PurgeOrphans deletes the items whose media item or music track no longer
	// exists (no FK: an item points at either) and returns how many went.
	PurgeOrphans(ctx context.Context) (int64, error)
}
