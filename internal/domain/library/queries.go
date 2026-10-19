package library

import (
	"context"
	"sort"

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
	// ListStreamFacets returns the distinct stream values of the items filter
	// selects, across the whole set rather than a sample of loaded items. The
	// filter's WHERE applies exactly as in List; its paging, cursor and sort
	// do not. The parental rule, when set, hides the same items as in List.
	ListStreamFacets(ctx context.Context, filter ListMediaItemsFilter) (StreamFacets, error)
	// CountPerLibrary is Count grouped by library, in one query: for each
	// library of filter.LibraryIDs, what Count answers with LibraryID set to
	// it. A library with no matching item is absent from the map, and an empty
	// LibraryIDs counts nothing (it would read as "every library" in Count).
	CountPerLibrary(ctx context.Context, filter ListMediaItemsFilter) (map[shared.LibraryID]int, error)
}

// StreamFacets holds the distinct values a library filter panel offers, raw
// as probed: clients normalize them for display with the same functions they
// match items with. Empty codecs and sizes without a height are left out, as
// are empty and "und" languages.
type StreamFacets struct {
	VideoCodecs       []string
	VideoSizes        []VideoSize
	AudioLanguages    []string
	SubtitleLanguages []string
}

// VideoSize is one distinct frame size of a video stream.
type VideoSize struct {
	Width  int
	Height int
}

// ResolutionTiers folds the frame sizes onto their distinct ResolutionTier,
// highest first: the bands a resolution filter offers.
func (f StreamFacets) ResolutionTiers() []int {
	seen := make(map[int]bool, len(f.VideoSizes))
	var tiers []int
	for _, s := range f.VideoSizes {
		t := ResolutionTier(s.Width, s.Height)
		if !seen[t] {
			seen[t] = true
			tiers = append(tiers, t)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(tiers)))
	return tiers
}
