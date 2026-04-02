package metadata

import (
	"context"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	domainlib "github.com/gravity-zero/evey/internal/domain/library"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
)

// AutoEnrichItem searches providers and enriches a single item.
// Primary provider first, then supplementary for extra IDs/ratings.
func (e *Enricher) AutoEnrichItem(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
	if item.Title == "" {
		return
	}

	if item.Type == string(domainlib.Episode) {
		e.autoEnrichEpisode(ctx, item, log)
		return
	}

	if item.Type == string(domainlib.Season) {
		e.autoEnrichSeason(ctx, item, log)
		return
	}

	if item.Overview != "" {
		return
	}

	mediaType := "movie"
	if item.Type == string(domainlib.Series) {
		mediaType = "tv"
	}

	// For series, count episodes in S01 on disk to validate TMDB matches
	var diskS01Episodes int
	if mediaType == "tv" {
		diskS01Episodes = e.countDiskEpisodesForSeason(ctx, item.ID, 1)
	}

	// 1. Try primary provider
	primary := e.searchAndFetchBest(ctx, e.primary, item.Title, item.Year, mediaType, diskS01Episodes, log)

	// 2. If primary failed, try supplementary as fallback
	if primary == nil {
		for _, sp := range e.supplementary {
			primary = e.searchAndFetchBest(ctx, sp, item.Title, item.Year, mediaType, diskS01Episodes, log)
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

	// 3b. Fetch and store translations in other locales
	if tmdbID := primary.ProviderIDs["tmdb"]; tmdbID != "" {
		e.enrichTranslations(ctx, item, tmdbID, mediaType)
	}

	// 4. Supplement with extra IDs, ratings from secondary providers
	e.supplementItem(ctx, item, primary, mediaType, log)
}

// supplementItem calls supplementary providers to merge extra data (IMDB ID, ratings, TheTVDB ID).
func (e *Enricher) supplementItem(ctx context.Context, item *domainlib.MediaItem, primary *domainmetadata.MetadataResult, mediaType string, log *slog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in supplementItem", slog.Any("recover", r), slog.String("title", item.Title))
		}
	}()
	imdbID := primary.ProviderIDs["imdb"]
	changed := false

	for _, sp := range e.supplementary {
		if sp.Name() == e.primary.Name() {
			continue
		}
		supplement := lookupSupplement(ctx, sp, imdbID, mediaType)
		if supplement == nil {
			continue
		}
		if mergeProviderIDs(item, supplement.ProviderIDs) {
			changed = true
		}
		if mergeRatings(item, supplement.Ratings) {
			changed = true
		}
	}

	if changed {
		item.UpdatedAt = time.Now()
		if err := e.mediaItems.Update(ctx, item); err != nil && log != nil {
			log.Warn("supplement update failed", slog.String("title", item.Title), slog.Any("err", err))
		}
	}
}

func lookupSupplement(ctx context.Context, sp domainmetadata.MetadataProvider, imdbID, mediaType string) *domainmetadata.MetadataResult {
	if imdbID == "" {
		return nil
	}
	if sp.Name() == "omdb" {
		s, err := sp.GetByProviderID(ctx, imdbID, mediaType)
		if err == nil && s != nil {
			return s
		}
	}
	if lookup, ok := sp.(domainmetadata.ExternalIDLookup); ok {
		s, err := lookup.LookupByExternalID(ctx, "imdb", imdbID)
		if err == nil && s != nil {
			return s
		}
	}
	return nil
}

func mergeProviderIDs(item *domainlib.MediaItem, src map[string]string) bool {
	changed := false
	for k, v := range src {
		if _, exists := item.ProviderIDs[k]; !exists {
			if item.ProviderIDs == nil {
				item.ProviderIDs = make(map[string]string)
			}
			item.ProviderIDs[k] = v
			changed = true
		}
	}
	return changed
}

func mergeRatings(item *domainlib.MediaItem, src map[string]float64) bool {
	changed := false
	for k, v := range src {
		if _, exists := item.Ratings[k]; !exists {
			if item.Ratings == nil {
				item.Ratings = make(map[string]float64)
			}
			item.Ratings[k] = v
			changed = true
		}
	}
	return changed
}

// countDiskEpisodesForSeason counts episodes on disk for a given season of a series.
func (e *Enricher) countDiskEpisodesForSeason(ctx context.Context, seriesID string, seasonNum int) int {
	if e.mediaItems == nil {
		return 0
	}
	seasonType := "season"
	seasons, err := e.mediaItems.List(ctx, domainlib.ListMediaItemsFilter{
		ParentID: &seriesID,
		Type:     &seasonType,
	})
	if err != nil {
		return 0
	}
	for _, s := range seasons {
		if s.IndexNumber != nil && *s.IndexNumber == seasonNum {
			episodeType := "episode"
			episodes, epErr := e.mediaItems.List(ctx, domainlib.ListMediaItemsFilter{
				ParentID: &s.ID,
				Type:     &episodeType,
			})
			if epErr != nil {
				return 0
			}
			return len(episodes)
		}
	}
	return 0
}

// searchAndFetchBest searches a provider and picks the best match, validating
// against disk structure (episode counts) for series.
func (e *Enricher) searchAndFetchBest(ctx context.Context, provider domainmetadata.MetadataProvider, title string, year *int, mediaType string, diskS01Episodes int, log *slog.Logger) *domainmetadata.MetadataResult {
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

	// For non-series or when we don't have disk episode info, use simple matching
	if mediaType != "tv" || diskS01Episodes == 0 {
		return e.fetchBestCandidate(ctx, provider, results, title, year, mediaType)
	}

	// For series: fetch details for top candidates and validate with combined
	// title similarity + episode count matching
	ranked := rankByTitle(results, title, year)
	maxCandidates := 5
	if len(ranked) < maxCandidates {
		maxCandidates = len(ranked)
	}

	type validatedMatch struct {
		result     *domainmetadata.MetadataResult
		titleScore float64
	}
	var validated []validatedMatch

	for _, candidate := range ranked[:maxCandidates] {
		providerID := candidate.ProviderIDs[provider.Name()]
		if providerID == "" {
			continue
		}
		full, fetchErr := provider.GetByProviderID(ctx, providerID, mediaType)
		if fetchErr != nil || full == nil {
			continue
		}
		if tmdbS01, ok := full.SeasonEpisodeCounts[1]; ok {
			if episodeCountMatches(diskS01Episodes, tmdbS01) {
				s1 := titleSimilarity(title, full.Title)
				s2 := titleSimilarity(title, full.OriginalTitle)
				best := s1
				if s2 > best {
					best = s2
				}
				validated = append(validated, validatedMatch{result: full, titleScore: best})
			}
		}
	}

	if len(validated) > 0 {
		// Pick the validated match with the highest title similarity
		best := validated[0]
		for _, v := range validated[1:] {
			if v.titleScore > best.titleScore {
				best = v
			}
		}
		if log != nil {
			log.Debug("series match validated by S01 episode count + title score",
				slog.String("title", best.result.Title),
				slog.Float64("title_score", best.titleScore),
				slog.Int("disk_episodes", diskS01Episodes))
		}
		return best.result
	}

	// Fallback: return the best title match even without episode validation
	if log != nil {
		log.Debug("no series match validated by episode count, using best title match", slog.String("title", title))
	}
	return e.fetchBestCandidate(ctx, provider, results, title, year, mediaType)
}

// fetchBestCandidate picks the best title match and fetches full details.
func (e *Enricher) fetchBestCandidate(ctx context.Context, provider domainmetadata.MetadataProvider, results []domainmetadata.MetadataResult, title string, year *int, mediaType string) *domainmetadata.MetadataResult {
	best := pickBestMatch(results, title, year)
	providerID := best.ProviderIDs[provider.Name()]
	if providerID == "" {
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

// episodeCountMatches returns true if counts are within 20% tolerance
// (some episodes may not have been downloaded yet).
func episodeCountMatches(disk, tmdb int) bool {
	if disk == 0 || tmdb == 0 {
		return false
	}
	diff := disk - tmdb
	if diff < 0 {
		diff = -diff
	}
	// Allow 20% tolerance — user may not have all episodes
	tolerance := float64(tmdb) * 0.2
	if tolerance < 1 {
		tolerance = 1
	}
	return float64(diff) <= tolerance
}

// rankByTitle sorts results by title similarity (best first).
func rankByTitle(results []domainmetadata.MetadataResult, query string, year *int) []domainmetadata.MetadataResult {
	type scored struct {
		result domainmetadata.MetadataResult
		score  float64
	}
	var items []scored
	for _, r := range results {
		s1 := titleSimilarity(query, r.Title)
		s2 := titleSimilarity(query, r.OriginalTitle)
		score := s1
		if s2 > score {
			score = s2
		}
		if year != nil && r.Year != nil && *r.Year == *year {
			score += 0.3
		}
		items = append(items, scored{result: r, score: score})
	}
	slices.SortFunc(items, func(a, b scored) int {
		if a.score != b.score {
			if a.score > b.score {
				return -1
			}
			return 1
		}
		if a.result.Popularity > b.result.Popularity {
			return -1
		}
		return 1
	})
	out := make([]domainmetadata.MetadataResult, len(items))
	for i, it := range items {
		out[i] = it.result
	}
	return out
}

// searchAndFetch searches a provider and fetches full details for the best match (legacy, used by supplementary).
func (e *Enricher) searchAndFetch(ctx context.Context, provider domainmetadata.MetadataProvider, title string, year *int, mediaType string, log *slog.Logger) *domainmetadata.MetadataResult {
	return e.searchAndFetchBest(ctx, provider, title, year, mediaType, 0, log)
}

func (e *Enricher) autoEnrichSeason(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
	if item.ParentID == "" || item.IndexNumber == nil {
		return
	}
	series, err := e.mediaItems.GetByID(ctx, item.ParentID)
	if err != nil {
		return
	}

	// Ensure series is enriched first
	if series.Overview == "" {
		e.AutoEnrichItem(ctx, series, log)
		series, err = e.mediaItems.GetByID(ctx, item.ParentID)
		if err != nil {
			return
		}
	}

	// Try each provider that supports season details
	for _, p := range append([]domainmetadata.MetadataProvider{e.primary}, e.supplementary...) {
		sp, ok := p.(domainmetadata.SeasonDetailProvider)
		if !ok {
			continue
		}
		seriesID := series.ProviderIDs[p.Name()]
		if seriesID == "" {
			continue
		}
		result, err := sp.GetSeasonDetails(ctx, seriesID, *item.IndexNumber)
		if err != nil {
			if log != nil {
				log.Debug("season lookup failed",
					slog.String("provider", p.Name()),
					slog.String("series_id", seriesID),
					slog.Int("season", *item.IndexNumber),
					slog.Any("err", err))
			}
			continue
		}
		if err := e.EnrichItem(ctx, item, result); err != nil && log != nil {
			log.Warn("season enrich failed", slog.String("id", item.ID), slog.Any("err", err))
		}
		return
	}
}

func (e *Enricher) autoEnrichEpisode(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
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

// AutoEnrichAll enriches all items concurrently (max 3 at a time)
// with rate limiting to avoid 429s from external APIs.
func (e *Enricher) AutoEnrichAll(ctx context.Context, items []*domainlib.MediaItem, log *slog.Logger) {
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, item := range items {
		if item.Overview != "" && len(item.Trailers) > 0 {
			continue
		}
		if err := e.apiLimiter.Wait(ctx); err != nil {
			return
		}
		wg.Add(1)
		go func(it *domainlib.MediaItem) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in metadata enricher", "error", r, "stack", debug.Stack())
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			e.AutoEnrichItem(ctx, it, log)
		}(item)
	}
	wg.Wait()
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
