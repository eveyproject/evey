package fakes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeMediaItemQueryRepo struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	cast  map[string][]library.CastMember
	Err   error
}

func NewFakeMediaItemQueryRepo() *FakeMediaItemQueryRepo {
	return &FakeMediaItemQueryRepo{
		items: make(map[string]*library.MediaItem),
		cast:  make(map[string][]library.CastMember),
	}
}

// Add seeds an item so GetByID/GetByIDs/List can return it in tests.
func (r *FakeMediaItemQueryRepo) Add(item *library.MediaItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[string(item.ID)] = cloneMediaItem(item)
}

func (r *FakeMediaItemQueryRepo) GetByID(_ context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	item, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneMediaItem(item), nil
}

func (r *FakeMediaItemQueryRepo) GetByFilePath(_ context.Context, path string) (*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.File != nil && item.File.Path == path {
			return cloneMediaItem(item), nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemQueryRepo) GetByLibraryTypeTitle(_ context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.LibraryID == libraryID && string(item.Type) == itemType && item.Title == title &&
			(parentID == "" || item.ParentID == parentID) {
			return cloneMediaItem(item), nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemQueryRepo) GetByIDs(_ context.Context, ids []shared.MediaItemID) ([]*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.MediaItem
	for _, id := range ids {
		if item, ok := r.items[string(id)]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

// matchesGenres mirrors the repo's OR-match: an item passes if its catalog
// genres include at least one of the requested genres. An empty filter matches
// everything.
func matchesGenres(item *library.MediaItem, genres []string) bool {
	if len(genres) == 0 {
		return true
	}
	if item.Catalog == nil {
		return false
	}
	have := make(map[string]bool, len(item.Catalog.Genres))
	for _, g := range item.Catalog.Genres {
		have[g] = true
	}
	for _, g := range genres {
		if have[g] {
			return true
		}
	}
	return false
}

func allowedRatingsSet(allowed []string) map[string]bool {
	if len(allowed) == 0 {
		return nil
	}
	m := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		m[strings.ToUpper(strings.TrimSpace(r))] = true
	}
	return m
}

func (r *FakeMediaItemQueryRepo) List(_ context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	allowedSet := allowedRatingsSet(filter.AllowedRatings)
	var result []*library.MediaItem
	libIDsSet := make(map[string]bool, len(filter.LibraryIDs))
	for _, id := range filter.LibraryIDs {
		libIDsSet[string(id)] = true
	}
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if len(libIDsSet) > 0 && !libIDsSet[string(item.LibraryID)] {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		if filter.Type != nil && string(item.Type) != *filter.Type {
			continue
		}
		if !matchesGenres(item, filter.Genres) {
			continue
		}
		if allowedSet != nil {
			official := ""
			if item.Classification != nil {
				official = item.Classification.OfficialRating
			}
			rating := strings.ToUpper(strings.TrimSpace(official))
			if rating != "" && !allowedSet[rating] {
				continue
			}
		}
		yr := catalogYear(item)
		if filter.YearFrom != 0 && (yr == nil || *yr < filter.YearFrom) {
			continue
		}
		if filter.YearTo != 0 && (yr == nil || *yr > filter.YearTo) {
			continue
		}
		if filter.MinRating != 0 {
			cr := classificationCommunityRating(item)
			if cr == nil || *cr < filter.MinRating {
				continue
			}
		}
		if filter.MissingProviderIDs != nil {
			hasPIDs := item.Catalog != nil && len(item.Catalog.ProviderIDs) > 0
			if *filter.MissingProviderIDs && hasPIDs {
				continue
			}
			if !*filter.MissingProviderIDs && !hasPIDs {
				continue
			}
		}
		result = append(result, item)
	}
	sortKey := filter.SortField
	if sortKey == "" {
		sortKey = "title"
	}
	desc := strings.EqualFold(filter.SortOrder, "desc")
	sortMediaItems(result, sortKey, desc)
	if filter.Cursor.LastID != "" {
		if filter.Cursor.SortField != sortKey {
			return nil, shared.ErrCursorMismatch
		}
		result = applyCursor(result, sortKey, filter.Cursor, desc)
	} else {
		if filter.Offset > 0 && filter.Offset < len(result) {
			result = result[filter.Offset:]
		} else if filter.Offset >= len(result) {
			return nil, nil
		}
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *FakeMediaItemQueryRepo) Count(_ context.Context, filter library.ListMediaItemsFilter) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return 0, r.Err
	}
	allowedSet := allowedRatingsSet(filter.AllowedRatings)
	countLibIDsSet := make(map[string]bool, len(filter.LibraryIDs))
	for _, id := range filter.LibraryIDs {
		countLibIDsSet[string(id)] = true
	}
	count := 0
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if len(countLibIDsSet) > 0 && !countLibIDsSet[string(item.LibraryID)] {
			continue
		}
		if filter.Type != nil && string(item.Type) != *filter.Type {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		if !matchesGenres(item, filter.Genres) {
			continue
		}
		if allowedSet != nil {
			official := ""
			if item.Classification != nil {
				official = item.Classification.OfficialRating
			}
			rating := strings.ToUpper(strings.TrimSpace(official))
			if rating != "" && !allowedSet[rating] {
				continue
			}
		}
		yr := catalogYear(item)
		if filter.YearFrom != 0 && (yr == nil || *yr < filter.YearFrom) {
			continue
		}
		if filter.YearTo != 0 && (yr == nil || *yr > filter.YearTo) {
			continue
		}
		if filter.MinRating != 0 {
			cr := classificationCommunityRating(item)
			if cr == nil || *cr < filter.MinRating {
				continue
			}
		}
		count++
	}
	return count, nil
}

func (r *FakeMediaItemQueryRepo) GetCastByMediaItem(_ context.Context, mediaItemID shared.MediaItemID) ([]library.CastMember, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return r.cast[string(mediaItemID)], nil
}

func (r *FakeMediaItemQueryRepo) GetCastForItems(_ context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]library.CastMember, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[shared.MediaItemID][]library.CastMember)
	for _, id := range mediaItemIDs {
		if cast, ok := r.cast[string(id)]; ok {
			result[id] = cast
		}
	}
	return result, nil
}

func (r *FakeMediaItemQueryRepo) ListGenres(_ context.Context, libraryID shared.LibraryID) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	set := make(map[string]bool)
	for _, item := range r.items {
		if item.LibraryID != libraryID || item.Catalog == nil {
			continue
		}
		for _, g := range item.Catalog.Genres {
			if g != "" {
				set[g] = true
			}
		}
	}
	genres := make([]string, 0, len(set))
	for g := range set {
		genres = append(genres, g)
	}
	sort.Strings(genres)
	return genres, nil
}

func (r *FakeMediaItemQueryRepo) SeriesAggregate(_ context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	if r.Err != nil {
		return library.SeriesAggregate{}, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return library.SeriesAggregate{}, r.Err
	}
	visited := map[string]bool{string(seriesID): true}
	queue := []string{string(seriesID)}
	var agg library.SeriesAggregate
	var totalMs int64
	seasons := make(map[string]bool)
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, item := range r.items {
			if item.ParentID != parent {
				continue
			}
			if visited[string(item.ID)] {
				continue
			}
			visited[string(item.ID)] = true
			queue = append(queue, string(item.ID))
			if string(item.Type) == "episode" {
				if item.File != nil && !item.File.Duration.IsZero() {
					totalMs += item.File.Duration.Milliseconds()
				}
				agg.EpisodeCount++
				if item.ParentID != "" {
					seasons[item.ParentID] = true
				}
			}
		}
	}
	agg.SeasonCount = len(seasons)
	dur, derr := shared.NewDurationMs(totalMs)
	if derr != nil {
		return library.SeriesAggregate{}, derr
	}
	agg.TotalDuration = dur
	return agg, nil
}

func classificationCommunityRating(m *library.MediaItem) *float64 {
	if m == nil || m.Classification == nil {
		return nil
	}
	return m.Classification.CommunityRating
}

func catalogYear(m *library.MediaItem) *int {
	if m == nil || m.Catalog == nil {
		return nil
	}
	return m.Catalog.Year
}

// itemSortValue returns the comparable string form of a media item's
// value for the given sort key. Mirrors what the postgres repo would
// place into a cursor's SortValue (always a string round-trip).
func itemSortValue(m *library.MediaItem, sortKey string) string {
	switch sortKey {
	case "title", "sort_title":
		if m.SortTitle != "" {
			return m.SortTitle
		}
		return m.Title
	case "added_at", "date_added":
		return m.AddedAt.UTC().Format(time.RFC3339Nano)
	case "year":
		if m.Catalog != nil && m.Catalog.Year != nil {
			return fmt.Sprintf("%010d", *m.Catalog.Year)
		}
		return ""
	case "premiere_date":
		if m.Catalog != nil && m.Catalog.PremiereDate != nil {
			return m.Catalog.PremiereDate.UTC().Format(time.RFC3339Nano)
		}
		return ""
	case "community_rating":
		if m.Classification != nil && m.Classification.CommunityRating != nil {
			return fmt.Sprintf("%020.6f", *m.Classification.CommunityRating)
		}
		return ""
	default:
		return string(m.ID)
	}
}

// sortKeyIsNullable reports whether an EMPTY itemSortValue means "this item has
// no value" rather than "its value is the empty string". Only these three reach
// SQL through a LEFT JOIN (media_catalog, media_classifications); title and
// added_at live on media_items and are always present.
func sortKeyIsNullable(sortKey string) bool {
	switch sortKey {
	case "year", "premiere_date", "community_rating":
		return true
	default:
		return false
	}
}

// sortsAsNull reports whether this item has no value for the sort key, i.e.
// whether Postgres would treat it as NULL when ordering.
func sortsAsNull(v, sortKey string) bool {
	return v == "" && sortKeyIsNullable(sortKey)
}

// Ordering mirrors Postgres, including where it puts NULLs: LAST on ASC, FIRST
// on DESC. Plain string comparison would do the opposite (an empty string sorts
// smallest), so a fake built on it would hand handler tests an order production
// never produces - the fake that "did not sort" once made a correct
// offset-to-cursor handover look broken for an entire session.
func sortMediaItems(items []*library.MediaItem, sortKey string, desc bool) {
	sort.SliceStable(items, func(i, j int) bool {
		vi, vj := itemSortValue(items[i], sortKey), itemSortValue(items[j], sortKey)
		ni, nj := sortsAsNull(vi, sortKey), sortsAsNull(vj, sortKey)
		if ni != nj {
			if desc {
				return ni // NULLs first
			}
			return nj // NULLs last
		}
		if vi == vj {
			if desc {
				return string(items[i].ID) > string(items[j].ID)
			}
			return string(items[i].ID) < string(items[j].ID)
		}
		if desc {
			return vi > vj
		}
		return vi < vj
	})
}

// applyCursor drops everything up to and including the cursor's row. items must
// already be sorted, so the first row that "passed" starts the remainder.
//
// The NULL block is a position in the ordering, not a value: an empty
// SortValue means the previous page ended inside it.
func applyCursor(items []*library.MediaItem, sortKey string, c shared.Cursor, desc bool) []*library.MediaItem {
	cursorAtNull := sortsAsNull(c.SortValue, sortKey)
	for i, it := range items {
		v := itemSortValue(it, sortKey)
		vNull := sortsAsNull(v, sortKey)
		var passed bool
		switch {
		case cursorAtNull && vNull:
			// Same block: only the id orders them.
			if desc {
				passed = string(it.ID) < c.LastID
			} else {
				passed = string(it.ID) > c.LastID
			}
		case cursorAtNull:
			// Rows WITH a value: after the NULL block on DESC, before it on ASC.
			passed = desc
		case vNull:
			// The NULL block: after the valued rows on ASC, before them on DESC.
			passed = !desc
		case v == c.SortValue:
			if desc {
				passed = string(it.ID) < c.LastID
			} else {
				passed = string(it.ID) > c.LastID
			}
		case desc:
			passed = v < c.SortValue
		default:
			passed = v > c.SortValue
		}
		if passed {
			return items[i:]
		}
	}
	return nil
}

// NextEpisodeOf is a tests-only stub. The predictive next-episode warm-up
// is exercised by the postgres repo + http handler tests; this fake just
// satisfies the interface so unrelated handler tests compile.
func (r *FakeMediaItemQueryRepo) NextEpisodeOf(_ context.Context, _ shared.MediaItemID) (*library.MediaItem, error) {
	return nil, nil
}

// Seed adds items for testing.
func (r *FakeMediaItemQueryRepo) Seed(item *library.MediaItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[string(item.ID)] = cloneMediaItem(item)
}

// SeedCast sets cast for a media item.
func (r *FakeMediaItemQueryRepo) SeedCast(mediaItemID string, cast []library.CastMember) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cast[mediaItemID] = cast
}
