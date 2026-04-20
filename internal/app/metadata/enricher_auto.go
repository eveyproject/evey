package metadata

import (
	"context"
	"log/slog"
	"math"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
)

// AutoEnrichItem searches providers and enriches a single item.
// Primary provider first, then supplementary for extra IDs/ratings.
func (e *Enricher) AutoEnrichItem(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
	if item.Title == "" {
		return
	}

	if item.Type == domainlib.Episode {
		e.autoEnrichEpisode(ctx, item, log)
		return
	}

	if item.Type == domainlib.Season {
		e.autoEnrichSeason(ctx, item, log)
		return
	}

	if item.Overview != "" {
		return
	}

	// Respect admin intent: if any field has been manually overridden, don't
	// auto-refresh. Admins force a refresh via the explicit RefreshMetadata RPC.
	if e.overrides != nil {
		if overrides, err := e.overrides.GetByMediaItem(ctx, string(item.ID)); err == nil && len(overrides) > 0 {
			if log != nil {
				log.Debug("skip auto-enrich: item has admin overrides", slog.String("id", string(item.ID)), slog.Int("overrides", len(overrides)))
			}
			return
		}
	}

	mediaType := "movie"
	if item.Type == domainlib.Series {
		mediaType = "tv"
	}

	// For series, count episodes in S01 on disk to validate TMDB matches
	var diskS01Episodes int
	if mediaType == "tv" {
		diskS01Episodes = e.countDiskEpisodesForSeason(ctx, string(item.ID), 1)
	}

	// ScanTitle is the stable filesystem-derived title; Title may have been
	// overwritten by a past (possibly wrong) TMDB match. Always search against
	// the scan title so a bad match can be corrected on re-enrichment.
	searchTitle := item.ScanTitle
	if searchTitle == "" {
		searchTitle = item.Title
	}

	var year *int
	if item.Catalog != nil {
		year = item.Catalog.Year
	}

	// Walk the chain: first provider to return a match wins. Covers both
	// the primary attempt and the supplementary fallback in one pass.
	primary := e.chain.FirstNonNil(func(p domainmetadata.MetadataProvider) *domainmetadata.MetadataResult {
		return e.searchAndFetchBest(ctx, p, searchTitle, year, mediaType, diskS01Episodes, log)
	})
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
	primaryName := ""
	if p := e.chain.Primary(); p != nil {
		primaryName = p.Name()
	}
	changed := false

	for _, sp := range e.chain.Supplementary() {
		if sp.Name() == primaryName {
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
		if item.Catalog == nil {
			item.Catalog = &domainlib.MediaCatalog{}
		}
		if _, exists := item.Catalog.ProviderIDs[k]; !exists {
			if item.Catalog.ProviderIDs == nil {
				item.Catalog.ProviderIDs = make(map[string]string)
			}
			item.Catalog.ProviderIDs[k] = v
			changed = true
		}
	}
	return changed
}

func mergeRatings(item *domainlib.MediaItem, src map[string]float64) bool {
	if len(src) == 0 {
		return false
	}
	if item.Classification == nil {
		item.Classification = &domainlib.MediaClassification{}
	}
	if item.Classification.ProviderRatings == nil {
		item.Classification.ProviderRatings = make(map[string]float64)
	}
	changed := false
	for k, v := range src {
		if _, exists := item.Classification.ProviderRatings[k]; !exists {
			item.Classification.ProviderRatings[k] = v
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
			seasonID := string(s.ID)
			episodes, epErr := e.mediaItems.List(ctx, domainlib.ListMediaItemsFilter{
				ParentID: &seasonID,
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

	// For series: iterate TMDB results in their order, fetch detail for the
	// top N, return the first one whose S01 episode count matches the disk.
	// If none match, pick the candidate that scores best on a
	// recency+popularity+episode-proximity tiebreaker instead of blindly
	// taking the first fetched. TMDB's popularity sort puts iconic older
	// titles (e.g. "It" 1990 miniseries) ahead of recent remakes; the
	// tiebreaker rescues cases like "It: Welcome to Derry" (2025) that
	// don't match episode counts exactly but are clearly the user's intent.
	qw := wordSet(title)
	maxCandidates := 5
	checked := 0
	var bestFallback *domainmetadata.MetadataResult
	var bestFallbackScore float64
	for _, candidate := range results {
		if checked >= maxCandidates {
			break
		}
		if !sharesAnyWord(qw, candidate.Title) && !sharesAnyWord(qw, candidate.OriginalTitle) {
			continue
		}
		providerID := candidate.ProviderIDs[provider.Name()]
		if providerID == "" {
			continue
		}
		checked++
		full, fetchErr := provider.GetByProviderID(ctx, providerID, mediaType)
		if fetchErr != nil || full == nil {
			continue
		}
		tmdbS01, ok := full.SeasonEpisodeCounts[1]
		if !ok {
			continue
		}
		if episodeCountMatches(diskS01Episodes, tmdbS01) {
			if log != nil {
				log.Debug("series match validated by S01 episode count + coherent title",
					slog.String("title", full.Title),
					slog.Int("disk_episodes", diskS01Episodes))
			}
			return full
		}
		score := seriesFallbackScore(diskS01Episodes, tmdbS01, full.Year, full.Popularity)
		if bestFallback == nil || score > bestFallbackScore {
			bestFallback = full
			bestFallbackScore = score
		}
	}

	// No candidate matched the disk episode count — use the best fallback
	// (closest episode count + most recent + most popular), else simple
	// pickBestMatch.
	if bestFallback != nil {
		if log != nil {
			log.Debug("no series match validated by episode count, using best fallback",
				slog.String("title", bestFallback.Title),
				slog.Float64("score", bestFallbackScore))
		}
		return bestFallback
	}
	if log != nil {
		log.Debug("no coherent candidate found, falling back to simple pickBestMatch", slog.String("title", title))
	}
	return e.fetchBestCandidate(ctx, provider, results, title, year, mediaType)
}

// seriesFallbackScore ranks TV series candidates when no episode count matches
// exactly. Higher is better. Gives heavy weight to episode-count proximity,
// boosts recent titles (post-2015), and uses TMDB popularity as a tiebreaker.
// Designed to rescue cases like "It" → 2025 "Welcome to Derry" where the
// 1990 miniseries outranks the remake by raw popularity.
func seriesFallbackScore(diskEp, tmdbEp int, year *int, popularity float64) float64 {
	diff := math.Abs(float64(diskEp - tmdbEp))
	epScore := 100.0 / (1.0 + diff)
	yearScore := 0.0
	if year != nil && *year >= 2015 {
		yearScore = float64(*year-2015) * 2.0
	}
	popScore := math.Min(popularity, 50.0)
	return epScore + yearScore + popScore
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

// searchAndFetch searches a provider and fetches full details for the best match (legacy, used by supplementary).
func (e *Enricher) searchAndFetch(ctx context.Context, provider domainmetadata.MetadataProvider, title string, year *int, mediaType string, log *slog.Logger) *domainmetadata.MetadataResult {
	return e.searchAndFetchBest(ctx, provider, title, year, mediaType, 0, log)
}

func (e *Enricher) autoEnrichSeason(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
	if item.ParentID == "" || item.IndexNumber == nil {
		return
	}
	series, err := e.mediaItems.GetByID(ctx, shared.MediaItemID(item.ParentID))
	if err != nil {
		return
	}

	// Ensure series is enriched first
	if series.Overview == "" {
		e.AutoEnrichItem(ctx, series, log)
		series, err = e.mediaItems.GetByID(ctx, shared.MediaItemID(item.ParentID))
		if err != nil {
			return
		}
	}

	// Try each provider that supports season details
	for _, cap := range e.chain.SeasonCapable() {
		if series.Catalog == nil {
			continue
		}
		seriesID := series.Catalog.ProviderIDs[cap.Provider.Name()]
		if seriesID == "" {
			continue
		}
		result, err := cap.Detail.GetSeasonDetails(ctx, seriesID, *item.IndexNumber)
		if err != nil {
			if log != nil {
				log.Debug("season lookup failed",
					slog.String("provider", cap.Provider.Name()),
					slog.String("series_id", seriesID),
					slog.Int("season", *item.IndexNumber),
					slog.Any("err", err))
			}
			continue
		}
		if err := e.EnrichItem(ctx, item, result); err != nil && log != nil {
			log.Warn("season enrich failed", slog.String("id", string(item.ID)), slog.Any("err", err))
		}
		return
	}
}

func (e *Enricher) autoEnrichEpisode(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
	if item.ParentID == "" || item.IndexNumber == nil {
		return
	}
	season, err := e.mediaItems.GetByID(ctx, shared.MediaItemID(item.ParentID))
	if err != nil || season.ParentID == "" || season.IndexNumber == nil {
		return
	}
	series, err := e.mediaItems.GetByID(ctx, shared.MediaItemID(season.ParentID))
	if err != nil {
		return
	}

	primaryName := ""
	if p := e.chain.Primary(); p != nil {
		primaryName = p.Name()
	}
	// Try each episode-capable provider
	for _, cap := range e.chain.EpisodeCapable() {
		seriesProviders := func() map[string]string {
			if series.Catalog == nil {
				return nil
			}
			return series.Catalog.ProviderIDs
		}
		seriesID := seriesProviders()[cap.Provider.Name()]
		if seriesID == "" {
			// For primary, try enriching the series first
			if cap.Provider.Name() == primaryName && series.Overview == "" {
				e.AutoEnrichItem(ctx, series, log)
				series, err = e.mediaItems.GetByID(ctx, shared.MediaItemID(season.ParentID))
				if err != nil {
					continue
				}
				seriesID = seriesProviders()[cap.Provider.Name()]
			}
			if seriesID == "" {
				continue
			}
		}

		result, err := cap.Detail.GetEpisodeDetails(ctx, seriesID, *season.IndexNumber, *item.IndexNumber)
		if err != nil {
			if log != nil {
				log.Debug("episode lookup failed",
					slog.String("provider", cap.Provider.Name()),
					slog.String("series_id", seriesID),
					slog.Int("season", *season.IndexNumber),
					slog.Int("ep", *item.IndexNumber),
					slog.Any("err", err))
			}
			continue
		}

		if err := e.EnrichItem(ctx, item, result); err != nil && log != nil {
			log.Warn("episode enrich failed", slog.String("id", string(item.ID)), slog.Any("err", err))
		}
		return
	}
}

// AutoEnrichAll enriches all items concurrently (max 3 at a time)
// with rate limiting to avoid 429s from external APIs.
func (e *Enricher) AutoEnrichAll(ctx context.Context, items []*domainlib.MediaItem, log *slog.Logger) {
	// Batch-load admin overrides once to avoid N queries in the hot loop.
	overridden := map[string]bool{}
	if e.overrides != nil && len(items) > 0 {
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ids = append(ids, string(it.ID))
		}
		if m, err := e.overrides.ListOverriddenMediaItemIDs(ctx, ids); err == nil {
			overridden = m
		}
	}
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, item := range items {
		if overridden[string(item.ID)] {
			continue
		}
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

// pickBestMatch trusts TMDB's own popularity+relevance ranking and returns
// the first result that shares at least one word with the query (coherence
// filter). If none do, it falls back to the top TMDB result rather than
// second-guessing TMDB with a local heuristic. When year is provided and a
// shared-word candidate matches it exactly, that candidate wins — remakes/
// reboots with the same title stay disambiguated.
func pickBestMatch(results []domainmetadata.MetadataResult, query string, year *int) domainmetadata.MetadataResult {
	if len(results) == 1 {
		return results[0]
	}
	qw := wordSet(query)
	firstCoherentIdx := -1
	for i, r := range results {
		if !sharesAnyWord(qw, r.Title) && !sharesAnyWord(qw, r.OriginalTitle) {
			continue
		}
		if firstCoherentIdx < 0 {
			firstCoherentIdx = i
		}
		if year != nil && r.Year != nil && *r.Year == *year {
			return r
		}
	}
	if firstCoherentIdx >= 0 {
		return results[firstCoherentIdx]
	}
	return results[0]
}

func sharesAnyWord(qw map[string]bool, candidate string) bool {
	if candidate == "" || len(qw) == 0 {
		return false
	}
	for w := range wordSet(candidate) {
		if qw[w] {
			return true
		}
	}
	return false
}

func wordSet(s string) map[string]bool {
	m := make(map[string]bool)
	for w := range strings.FieldsSeq(strings.ToLower(s)) {
		t := strings.Trim(w, ".,:;!?'\"()[]{}«»—–-")
		if t != "" {
			m[t] = true
		}
	}
	return m
}
