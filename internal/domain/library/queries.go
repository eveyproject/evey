package library

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type LibraryQueryRepo interface {
	List(ctx context.Context, filter ListLibrariesFilter) ([]*Library, error)
	GetByID(ctx context.Context, id shared.LibraryID) (*Library, error)
}

type CastMember struct {
	PersonID      string
	PersonName    string
	ThumbPath     *string
	RoleType      string
	CharacterName *string
	SortOrder     int
}

type MediaItemQueryRepo interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*MediaItem, error)
	GetByIDs(ctx context.Context, ids []shared.MediaItemID) ([]*MediaItem, error)
	// GetByFilePath resolves the media item whose underlying file lives at path.
	// Read path used to gate filesystem-path operations (e.g. MKV chapter
	// lookup) by library access. Returns shared.ErrNotFound when no item matches.
	GetByFilePath(ctx context.Context, path string) (*MediaItem, error)
	// GetByLibraryTypeTitle resolves a media item by its (library, type,
	// title, parent) tuple. A pure lookup with no write-side caller today
	// (unlike GetByFilePath/GetByFileFingerprint, it does not gate a Create in
	// the scanner), so it belongs on the query side per ADR-0009.
	GetByLibraryTypeTitle(ctx context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*MediaItem, error)
	List(ctx context.Context, filter ListMediaItemsFilter) ([]*MediaItem, error)
	Count(ctx context.Context, filter ListMediaItemsFilter) (int, error)
	GetCastByMediaItem(ctx context.Context, mediaItemID shared.MediaItemID) ([]CastMember, error)
	// GetCastForItems fetches cast for multiple media items in one query (batch to avoid N+1).
	GetCastForItems(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]CastMember, error)
	SeriesAggregate(ctx context.Context, seriesID shared.MediaItemID) (SeriesAggregate, error)
	// NextEpisodeOf returns the next episode in the same series as the
	// given episode (next in same season, or first of next season if
	// current is the last of its season). Returns nil with no error when
	// the episode is the series finale. Used by the predictive warm-up
	// path: when the player is ~85% through an episode, we pre-spawn the
	// next one's transcode so the autoplay handoff is instant.
	NextEpisodeOf(ctx context.Context, episodeID shared.MediaItemID) (*MediaItem, error)
	// ListGenres returns the distinct, sorted genres present across a library's
	// media - the full set, not a sample of loaded items.
	ListGenres(ctx context.Context, libraryID shared.LibraryID) ([]string, error)
}
