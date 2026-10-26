package fakes

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeMediaItemQueryRepo struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	cast  map[string][]library.CastMember
	// streams backs ListStreamFacets only; seeded with SeedStreams.
	streams map[string][]*library.MediaStream
	Err     error
	// ErrList fails List alone, leaving GetByID and Count answering.
	ErrList error
	// ParentalBlocks applies a filter's ParentalRule (the rule is
	// ratings.Blocks, which a domain package may not import). A filter
	// carrying a rule while this is nil fails rather than ignore it.
	ParentalBlocks func(library.ParentalRule, *library.MediaItem) bool
}

// errParentalUnwired is the answer to a ParentalRule with no ParentalBlocks.
var errParentalUnwired = errors.New("fake media item repo: filter.Parental set but ParentalBlocks is nil")

func NewFakeMediaItemQueryRepo() *FakeMediaItemQueryRepo {
	return &FakeMediaItemQueryRepo{
		items:   make(map[string]*library.MediaItem),
		cast:    make(map[string][]library.CastMember),
		streams: make(map[string][]*library.MediaStream),
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
	if filter.Parental != nil && r.ParentalBlocks == nil {
		return nil, errParentalUnwired
	}
	if r.Err != nil {
		return nil, r.Err
	}
	if r.ErrList != nil {
		return nil, r.ErrList
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
		if filter.Parental != nil && r.ParentalBlocks(*filter.Parental, item) {
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
	// Keyset by id, as Postgres does: ORDER BY id, id > AfterID.
	if filter.KeysetByID || filter.AfterID != "" {
		sort.SliceStable(result, func(i, j int) bool { return result[i].ID < result[j].ID })
		if filter.AfterID != "" {
			kept := result[:0]
			for _, it := range result {
				if string(it.ID) > filter.AfterID {
					kept = append(kept, it)
				}
			}
			result = kept
		}
	} else if filter.Cursor.LastID != "" {
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
	// Postgres does NOT return everything when the caller leaves Limit at zero:
	// it clamps to DefaultListLimit. A fake that hands back the whole set makes
	// an unpaginated sweep look correct in every unit test and truncated in
	// production - which is what happened to RefreshAllTranslations. The number
	// is duplicated rather than imported (domain layer, postgres is an adapter).
	const defaultListLimit = 500 // mirrors postgres.DefaultListLimit
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *FakeMediaItemQueryRepo) Count(_ context.Context, filter library.ListMediaItemsFilter) (int, error) {
	if filter.Parental != nil && r.ParentalBlocks == nil {
		return 0, errParentalUnwired
	}
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
		if filter.Parental != nil && r.ParentalBlocks(*filter.Parental, item) {
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

// ListStreamFacets selects items through List (same predicates, parental
// rule included, no paging) and folds their seeded streams with the postgres
// rules: no empty codec, no size without a height, no empty or "und"
// language; sorted like the query.
func (r *FakeMediaItemQueryRepo) ListStreamFacets(ctx context.Context, filter library.ListMediaItemsFilter) (library.StreamFacets, error) {
	var facets library.StreamFacets
	if r.Err != nil {
		return facets, r.Err
	}
	f := filter
	f.Limit, f.Offset, f.Cursor = math.MaxInt32, 0, shared.Cursor{}
	items, err := r.List(ctx, f)
	if err != nil {
		return facets, err
	}
	codecs, sizes, audio, subs := map[string]bool{}, map[library.VideoSize]bool{}, map[string]bool{}, map[string]bool{}
	r.mu.RLock()
	for _, item := range items {
		for _, st := range r.streams[string(item.ID)] {
			switch st.StreamType {
			case library.StreamTypeVideo:
				if st.Codec != "" {
					codecs[st.Codec] = true
				}
				if st.Height != nil && *st.Height > 0 {
					w := 0
					if st.Width != nil {
						w = *st.Width
					}
					sizes[library.VideoSize{Width: w, Height: *st.Height}] = true
				}
			case library.StreamTypeAudio, library.StreamTypeSubtitle:
				if st.Language == "" || st.Language == "und" {
					continue
				}
				if st.StreamType == library.StreamTypeAudio {
					audio[st.Language] = true
				} else {
					subs[st.Language] = true
				}
			}
		}
	}
	r.mu.RUnlock()
	facets.VideoCodecs = sortedKeys(codecs)
	facets.AudioLanguages = sortedKeys(audio)
	facets.SubtitleLanguages = sortedKeys(subs)
	for sz := range sizes {
		facets.VideoSizes = append(facets.VideoSizes, sz)
	}
	sort.Slice(facets.VideoSizes, func(i, j int) bool {
		a, b := facets.VideoSizes[i], facets.VideoSizes[j]
		return a.Width < b.Width || (a.Width == b.Width && a.Height < b.Height)
	})
	return facets, nil
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// CountPerLibrary answers Count once per library of filter.LibraryIDs, which
// is exactly the contract the grouped postgres query has to meet.
func (r *FakeMediaItemQueryRepo) CountPerLibrary(ctx context.Context, filter library.ListMediaItemsFilter) (map[shared.LibraryID]int, error) {
	counts := make(map[shared.LibraryID]int, len(filter.LibraryIDs))
	for _, id := range filter.LibraryIDs {
		f := filter
		libID := id
		f.LibraryID, f.LibraryIDs = &libID, nil
		n, err := r.Count(ctx, f)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			counts[id] = n
		}
	}
	return counts, nil
}

func (r *FakeMediaItemQueryRepo) DescendantIDs(_ context.Context, id shared.MediaItemID) ([]shared.MediaItemID, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []shared.MediaItemID
	queue := []string{string(id)}
	visited := map[string]bool{string(id): true}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, item := range r.items {
			if item.ParentID != parent || visited[string(item.ID)] {
				continue
			}
			visited[string(item.ID)] = true
			queue = append(queue, string(item.ID))
			out = append(out, item.ID)
		}
	}
	return out, nil
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
	case "index_number":
		if m.IndexNumber != nil {
			return fmt.Sprintf("%010d", *m.IndexNumber)
		}
		return ""
	default:
		return string(m.ID)
	}
}

// sortKeyIsNullable reports whether an EMPTY itemSortValue means "this item has
// no value" rather than "its value is the empty string". Three reach SQL through
// a LEFT JOIN (media_catalog, media_classifications), and the episode index is a
// nullable media_items column; title and added_at are always present.
func sortKeyIsNullable(sortKey string) bool {
	switch sortKey {
	case "year", "premiere_date", "community_rating", "index_number":
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

// SeedStreams sets the streams ListStreamFacets reads for a media item.
func (r *FakeMediaItemQueryRepo) SeedStreams(mediaItemID string, streams []*library.MediaStream) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streams[mediaItemID] = streams
}

// SeedCast sets cast for a media item.
func (r *FakeMediaItemQueryRepo) SeedCast(mediaItemID string, cast []library.CastMember) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cast[mediaItemID] = cast
}
