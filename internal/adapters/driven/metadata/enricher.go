package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	domaindomain "github.com/gravity-zero/evey/internal/domain/library"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
)

// CollectionLinker upserts provider-sourced collections and links media items.
type CollectionLinker interface {
	Upsert(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*domaindomain.Collection, error)
	LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
}

type Enricher struct {
	artwork       domainmetadata.ArtworkRepository
	persons       domaindomain.PersonRepository
	mediaPersons  domaindomain.MediaPersonRepository
	mediaItems    domaindomain.MediaItemRepository
	search        domaindomain.SearchIndex
	primary       domainmetadata.MetadataProvider
	supplementary []domainmetadata.MetadataProvider
	collections   CollectionLinker
}

func NewEnricher(
	artwork domainmetadata.ArtworkRepository,
	persons domaindomain.PersonRepository,
	mediaPersons domaindomain.MediaPersonRepository,
	mediaItems domaindomain.MediaItemRepository,
	search domaindomain.SearchIndex,
	primary domainmetadata.MetadataProvider,
	supplementary []domainmetadata.MetadataProvider,
	collections CollectionLinker,
) *Enricher {
	return &Enricher{
		artwork:       artwork,
		persons:       persons,
		mediaPersons:  mediaPersons,
		mediaItems:    mediaItems,
		search:        search,
		primary:       primary,
		supplementary: supplementary,
		collections:   collections,
	}
}

// EnrichItem applies a MetadataResult to a media item: updates the item,
// replaces artwork records, and upserts people/cast.
func (e *Enricher) EnrichItem(ctx context.Context, item *domaindomain.MediaItem, result *domainmetadata.MetadataResult) error {
	item.Title = result.Title
	item.OriginalTitle = result.OriginalTitle
	item.Overview = result.Overview
	item.Tagline = result.Tagline
	item.Genres = result.Genres
	item.Studios = result.Studios
	item.PremiereDate = result.PremiereDate
	item.Year = result.Year
	item.CommunityRating = result.CommunityRating
	item.OfficialRating = result.OfficialRating
	item.Trailers = make([]domaindomain.Trailer, 0, len(result.Trailers))
	for _, t := range result.Trailers {
		item.Trailers = append(item.Trailers, domaindomain.Trailer{
			Site:             t.Site,
			Key:              t.Key,
			Language:         t.Language,
			Official:         t.Official,
			Type:             t.Type,
			UseSubtitles:     t.UseSubtitles,
			SubtitleLanguage: t.SubtitleLanguage,
		})
	}
	item.UpdatedAt = time.Now()

	if item.ProviderIDs == nil {
		item.ProviderIDs = make(map[string]string)
	}
	maps.Copy(item.ProviderIDs, result.ProviderIDs)

	if item.Ratings == nil {
		item.Ratings = make(map[string]float64)
	}
	maps.Copy(item.Ratings, result.Ratings)

	if err := e.mediaItems.Update(ctx, item); err != nil {
		return fmt.Errorf("update media item: %w", err)
	}

	if err := e.artwork.DeleteByMediaItemID(ctx, item.ID); err != nil {
		return fmt.Errorf("delete old artwork: %w", err)
	}
	for i, art := range result.Artwork {
		src := art.URL
		lang := art.Language
		w, h := art.Width, art.Height
		provider := art.Provider
		if provider == "" {
			provider = "unknown"
		}
		rec := &domainmetadata.ArtworkRecord{
			ID:          uuid.New().String(),
			MediaItemID: item.ID,
			Type:        art.Type,
			Provider:    provider,
			SourceURL:   &src,
			SortOrder:   i,
		}
		if lang != "" {
			rec.Language = &lang
		}
		if w > 0 {
			rec.Width = &w
		}
		if h > 0 {
			rec.Height = &h
		}
		if err := e.artwork.Create(ctx, rec); err != nil {
			return fmt.Errorf("create artwork: %w", err)
		}
	}

	if err := e.mediaPersons.DeleteByMediaItemID(ctx, item.ID); err != nil {
		return fmt.Errorf("delete old media_people: %w", err)
	}
	seenPersons := make(map[string]bool)
	sortIdx := 0
	for _, p := range result.People {
		if seenPersons[p.Name] {
			continue
		}
		seenPersons[p.Name] = true

		person, err := e.persons.GetByName(ctx, p.Name)
		if err != nil {
			if !errors.Is(err, domaindomain.ErrNotFound) {
				return fmt.Errorf("get person by name: %w", err)
			}
			thumbPath := p.ThumbURL
			var thumbPtr *string
			if thumbPath != "" {
				thumbPtr = &thumbPath
			}
			person = &domaindomain.Person{
				ID:          uuid.New().String(),
				Name:        p.Name,
				ProviderIDs: map[string]string{},
				ThumbPath:   thumbPtr,
			}
			if err := e.persons.Create(ctx, person); err != nil {
				return fmt.Errorf("create person: %w", err)
			}
		}

		var charName *string
		if p.Character != "" {
			c := p.Character
			charName = &c
		}
		mp := &domaindomain.MediaPerson{
			MediaItemID:   item.ID,
			PersonID:      person.ID,
			RoleType:      roleType(p.Role),
			CharacterName: charName,
			SortOrder:     sortIdx,
		}
		sortIdx++
		if createErr := e.mediaPersons.Create(ctx, mp); createErr != nil {
			slog.Warn("create media person failed", slog.String("personID", person.ID), slog.String("mediaItemID", item.ID), slog.Any("err", createErr))
		}
	}

	if e.search != nil {
		names := make([]string, 0, len(result.People))
		for _, p := range result.People {
			names = append(names, p.Name)
		}
		if err := e.search.IndexMediaItem(ctx, item, names); err != nil {
			slog.Warn("index media item", slog.Any("err", err))
		}
	}

	if e.collections != nil && result.Collection != nil {
		ref := result.Collection
		col, err := e.collections.Upsert(ctx, ref.TmdbID, ref.Name, "", ref.PosterURL, ref.BackdropURL)
		if err != nil {
			slog.Warn("upsert collection", slog.Int("tmdb_id", ref.TmdbID), slog.Any("err", err))
		} else if col != nil {
			if err := e.collections.LinkMediaItem(ctx, col.ID, item.ID, 0); err != nil {
				slog.Warn("link media item to collection", slog.String("collection_id", col.ID), slog.String("media_item_id", item.ID), slog.Any("err", err))
			}
		}
	}

	return nil
}

func roleType(role string) string {
	switch role {
	case "Actor":
		return "actor"
	case "Director":
		return "director"
	case "Writer":
		return "writer"
	case "Producer":
		return "producer"
	default:
		return "actor"
	}
}

// AutoEnrichItem searches providers and enriches a single item.
// Primary provider first, then supplementary for extra IDs/ratings.
func (e *Enricher) AutoEnrichItem(ctx context.Context, item *domaindomain.MediaItem, log *slog.Logger) {
	if item.Title == "" {
		return
	}

	if item.Type == string(domaindomain.Episode) {
		e.autoEnrichEpisode(ctx, item, log)
		return
	}

	if item.Overview != "" {
		return
	}

	mediaType := "movie"
	if item.Type == string(domaindomain.Series) {
		mediaType = "tv"
	}

	// 1. Try primary provider
	primary := e.searchAndFetch(ctx, e.primary, item.Title, item.Year, mediaType, log)

	// 2. If primary failed, try supplementary as fallback
	if primary == nil {
		for _, sp := range e.supplementary {
			primary = e.searchAndFetch(ctx, sp, item.Title, item.Year, mediaType, log)
			if primary != nil {
				break
			}
		}
	}

	if primary == nil {
		return
	}

	// 3. Enrich with primary result first (saves to DB immediately)
	if err := e.EnrichItem(ctx, item, primary); err != nil && log != nil {
		log.Warn("auto-enrich failed", slog.String("title", item.Title), slog.Any("err", err))
		return
	}

	// 4. Supplement async (extra IDs, ratings) — does not block
	go e.supplementItem(context.Background(), item, primary, mediaType, log)
}

// supplementItem calls supplementary providers to merge extra data (IMDB ID, ratings, TheTVDB ID).
func (e *Enricher) supplementItem(ctx context.Context, item *domaindomain.MediaItem, primary *domainmetadata.MetadataResult, mediaType string, log *slog.Logger) {
	imdbID := primary.ProviderIDs["imdb"]
	changed := false

	for _, sp := range e.supplementary {
		if sp.Name() == e.primary.Name() {
			continue
		}

		var supplement *domainmetadata.MetadataResult

		// Try lookup by IMDB ID if available
		if imdbID != "" {
			if sp.Name() == "omdb" {
				s, err := sp.GetByProviderID(ctx, imdbID, mediaType)
				if err == nil && s != nil {
					supplement = s
				}
			}
			if lookup, ok := sp.(domainmetadata.ExternalIDLookup); ok {
				s, err := lookup.LookupByExternalID(ctx, "imdb", imdbID)
				if err == nil && s != nil {
					supplement = s
				}
			}
		}

		if supplement == nil {
			continue
		}

		// Merge provider IDs
		for k, v := range supplement.ProviderIDs {
			if _, exists := item.ProviderIDs[k]; !exists {
				if item.ProviderIDs == nil {
					item.ProviderIDs = make(map[string]string)
				}
				item.ProviderIDs[k] = v
				changed = true
			}
		}

		// Merge ratings
		for k, v := range supplement.Ratings {
			if _, exists := item.Ratings[k]; !exists {
				if item.Ratings == nil {
					item.Ratings = make(map[string]float64)
				}
				item.Ratings[k] = v
				changed = true
			}
		}
	}

	if changed {
		item.UpdatedAt = time.Now()
		if err := e.mediaItems.Update(ctx, item); err != nil && log != nil {
			log.Warn("supplement update failed", slog.String("title", item.Title), slog.Any("err", err))
		}
	}
}

// searchAndFetch searches a provider and fetches full details for the best match.
func (e *Enricher) searchAndFetch(ctx context.Context, provider domainmetadata.MetadataProvider, title string, year *int, mediaType string, log *slog.Logger) *domainmetadata.MetadataResult {
	results, err := provider.Search(ctx, title, year, mediaType)
	if err != nil {
		if log != nil {
			log.Debug("provider search failed", slog.String("provider", provider.Name()), slog.String("title", title), slog.Any("err", err))
		}
		return nil
	}
	if len(results) == 0 {
		return nil
	}

	best := pickBestMatch(results, title, year)

	// Get the provider's own ID to fetch full details
	providerID := best.ProviderIDs[provider.Name()]
	if providerID == "" {
		// Try imdb ID for OMDb
		if provider.Name() == "omdb" {
			providerID = best.ProviderIDs["imdb"]
		}
		if providerID == "" {
			return &best
		}
	}

	full, err := provider.GetByProviderID(ctx, providerID, mediaType)
	if err != nil {
		return &best
	}
	return full
}

func (e *Enricher) autoEnrichEpisode(ctx context.Context, item *domaindomain.MediaItem, log *slog.Logger) {
	if item.ParentID == "" || item.IndexNumber == nil {
		return
	}
	season, err := e.mediaItems.GetByID(ctx, item.ParentID)
	if err != nil || season.ParentID == "" || season.IndexNumber == nil {
		return
	}
	series, err := e.mediaItems.GetByID(ctx, season.ParentID)
	if err != nil {
		return
	}

	// Try each episode-capable provider
	providers := e.episodeProviders()
	for _, ep := range providers {
		seriesID := series.ProviderIDs[ep.provider.Name()]
		if seriesID == "" {
			// For primary, try enriching the series first
			if ep.provider.Name() == e.primary.Name() && series.Overview == "" {
				e.AutoEnrichItem(ctx, series, log)
				series, err = e.mediaItems.GetByID(ctx, season.ParentID)
				if err != nil {
					continue
				}
				seriesID = series.ProviderIDs[ep.provider.Name()]
			}
			if seriesID == "" {
				continue
			}
		}

		result, err := ep.detail.GetEpisodeDetails(ctx, seriesID, *season.IndexNumber, *item.IndexNumber)
		if err != nil {
			if log != nil {
				log.Debug("episode lookup failed",
					slog.String("provider", ep.provider.Name()),
					slog.String("series_id", seriesID),
					slog.Int("season", *season.IndexNumber),
					slog.Int("ep", *item.IndexNumber),
					slog.Any("err", err))
			}
			continue
		}

		if err := e.EnrichItem(ctx, item, result); err != nil && log != nil {
			log.Warn("episode enrich failed", slog.String("id", item.ID), slog.Any("err", err))
		}
		return
	}
}

type episodeProvider struct {
	provider domainmetadata.MetadataProvider
	detail   domainmetadata.EpisodeDetailProvider
}

func (e *Enricher) episodeProviders() []episodeProvider {
	var result []episodeProvider
	if ep, ok := e.primary.(domainmetadata.EpisodeDetailProvider); ok {
		result = append(result, episodeProvider{provider: e.primary, detail: ep})
	}
	for _, sp := range e.supplementary {
		if ep, ok := sp.(domainmetadata.EpisodeDetailProvider); ok {
			result = append(result, episodeProvider{provider: sp, detail: ep})
		}
	}
	return result
}

// AutoEnrichAll enriches all items concurrently (max 3 at a time).
func (e *Enricher) AutoEnrichAll(ctx context.Context, items []*domaindomain.MediaItem, log *slog.Logger) {
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, item := range items {
		if item.Overview != "" {
			continue
		}
		wg.Add(1)
		go func(it *domaindomain.MediaItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			e.AutoEnrichItem(ctx, it, log)
		}(item)
	}
	wg.Wait()
}

// MergeResults merges supplementary results into a primary result.
// Primary fields are kept; supplementary only fills gaps.
func MergeResults(primary *domainmetadata.MetadataResult, supplements ...*domainmetadata.MetadataResult) *domainmetadata.MetadataResult {
	if primary.ProviderIDs == nil {
		primary.ProviderIDs = make(map[string]string)
	}
	if primary.Ratings == nil {
		primary.Ratings = make(map[string]float64)
	}

	for _, s := range supplements {
		if s == nil {
			continue
		}
		for k, v := range s.ProviderIDs {
			if _, exists := primary.ProviderIDs[k]; !exists {
				primary.ProviderIDs[k] = v
			}
		}
		for k, v := range s.Ratings {
			if _, exists := primary.Ratings[k]; !exists {
				primary.Ratings[k] = v
			}
		}
		if primary.Overview == "" {
			primary.Overview = s.Overview
		}
		if len(primary.People) == 0 {
			primary.People = s.People
		}
		if len(primary.Genres) == 0 {
			primary.Genres = s.Genres
		}
		if primary.CommunityRating == nil && s.CommunityRating != nil {
			primary.CommunityRating = s.CommunityRating
		}
		if primary.OfficialRating == "" {
			primary.OfficialRating = s.OfficialRating
		}
		// Append supplementary artwork (different providers = different images)
		primary.Artwork = append(primary.Artwork, s.Artwork...)
	}

	return primary
}

// pickBestMatch selects the result whose title best matches the query.
// Uses word overlap (Dice coefficient) on both Title and OriginalTitle.
// Year matching: exact match gets +0.3 bonus, close match (±2y) gets +0.1,
// large gap (>5y) gets -0.15 penalty to avoid reboots/remakes confusion.
func pickBestMatch(results []domainmetadata.MetadataResult, query string, year *int) domainmetadata.MetadataResult {
	if len(results) == 1 {
		return results[0]
	}

	bestIdx := 0
	bestScore := -1.0

	for i, r := range results {
		s1 := titleSimilarity(query, r.Title)
		s2 := titleSimilarity(query, r.OriginalTitle)
		score := s1
		if s2 > score {
			score = s2
		}
		if year != nil && r.Year != nil {
			diff := *r.Year - *year
			if diff < 0 {
				diff = -diff
			}
			switch {
			case diff == 0:
				score += 0.3
			case diff <= 2:
				score += 0.1
			default:
				// Proportional penalty: farther years get more penalty
				penalty := 0.03 * float64(diff)
				if penalty > 0.4 {
					penalty = 0.4
				}
				score -= penalty
			}
		}
		if score > bestScore || (score == bestScore && r.Popularity > results[bestIdx].Popularity) {
			bestScore = score
			bestIdx = i
		}
	}

	return results[bestIdx]
}

// titleSimilarity returns a 0..1 score using Dice coefficient on word sets.
func titleSimilarity(query, candidate string) float64 {
	if candidate == "" {
		return 0
	}
	qw := wordSet(query)
	cw := wordSet(candidate)
	if len(qw) == 0 || len(cw) == 0 {
		return 0
	}
	intersection := 0
	for w := range qw {
		if cw[w] {
			intersection++
		}
	}
	return 2.0 * float64(intersection) / float64(len(qw)+len(cw))
}

func wordSet(s string) map[string]bool {
	m := make(map[string]bool)
	for w := range strings.FieldsSeq(strings.ToLower(s)) {
		m[w] = true
	}
	return m
}
