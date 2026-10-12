package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// AutoEnrichItem searches providers and enriches a single item.
// Primary provider first, then supplementary for extra IDs/ratings.
// Skips items that already have an overview (considered enriched) - admin
// triggers a forced re-search via ForceAutoEnrichItem instead.
func (e *Enricher) AutoEnrichItem(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.AutoEnrichItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(item.ID)))
	e.autoEnrichItem(ctx, item, log, false, nil)
}

// ForceAutoEnrichItem bypasses the "already enriched" short-circuits so an
// explicit admin refresh can correct a past bad TMDB match. Admin field
// overrides are still respected - they represent manual intent the user
// doesn't want overwritten by an automated search.
func (e *Enricher) ForceAutoEnrichItem(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ForceAutoEnrichItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(item.ID)))
	e.autoEnrichItem(ctx, item, log, true, nil)
}

// autoEnrichItem searches providers and enriches a single item.
// overridesByItem is the batch-preloaded override map keyed by media item id
// (see autoEnrichAll): when non-nil, the per-item admin-override lookup below
// is skipped in favor of a map read, avoiding an N+1 GetByMediaItem call per
// item on a bulk "Refresh all metadata" pass. nil means the caller has no
// preload (e.g. the single-item AutoEnrichItem/ForceAutoEnrichItem entry
// points) and the legacy per-item fetch runs instead.
func (e *Enricher) autoEnrichItem(ctx context.Context, item *domainlib.MediaItem, log *slog.Logger, force bool, overridesByItem map[string][]*domainmetadata.MetadataOverride) {
	if item.Title == "" {
		return
	}

	if item.Type == domainlib.Episode {
		if !force && item.Overview != "" {
			return
		}
		e.autoEnrichEpisode(ctx, item, log)
		return
	}

	if item.Type == domainlib.Season {
		if !force && item.Overview != "" {
			return
		}
		e.autoEnrichSeason(ctx, item, log)
		return
	}

	if !force && item.Overview != "" {
		return
	}

	// Respect admin intent: manual field edits win, even under force refresh -
	// EXCEPT for orphan items (no TMDB/IMDB match yet). For orphans, the user
	// editing the title IS the request to find a match (otherwise their edit
	// just rots in the override table forever). Matched items continue to be
	// protected: their overrides are deliberate display preferences that
	// must not bounce back against the provider chain.
	isOrphan := item.Catalog == nil || len(item.Catalog.ProviderIDs) == 0
	var titleOverride, originalTitleOverride string
	if e.overrides != nil {
		var overrides []*domainmetadata.MetadataOverride
		var err error
		if overridesByItem != nil {
			// Preloaded by the caller in one batched GetByMediaItems call -
			// skip the per-item round trip.
			overrides = overridesByItem[string(item.ID)]
		} else {
			overrides, err = e.overrides.GetByMediaItem(ctx, string(item.ID))
		}
		if err != nil {
			// Fail safe, not fail open: the safe answer to "did an admin edit
			// this item?" is yes. Reading the error as "no overrides" let a
			// transient blip re-run the whole provider chain over an item
			// somebody deliberately corrected - the override rows survive and
			// the grid still overlays them, but the stored title and overview
			// are replaced underneath, and the search index (which serves the
			// provider title directly) then disagrees with every other screen.
			if log != nil {
				log.Warn("skip auto-enrich: admin overrides unreadable",
					slog.String("id", string(item.ID)), slog.Any("err", err))
			}
			return
		}
		if len(overrides) > 0 {
			if !isOrphan {
				if log != nil {
					log.Debug("skip auto-enrich: matched item has admin overrides", slog.String("id", string(item.ID)), slog.Int("overrides", len(overrides)))
				}
				return
			}
			// Orphan: extract title/original_title overrides for the search
			// query. Field names match what MediaDetailEdit writes (the web
			// FIELD_MAP uses snake_case for original_title - see
			// library_handler.go:986 for the matching read side).
			for _, ov := range overrides {
				if s, ok := ov.CustomValue.(string); ok && s != "" {
					switch ov.FieldName {
					case "title":
						titleOverride = s
					case "original_title":
						originalTitleOverride = s
					}
				}
			}
		}
	}

	mediaType := "movie"
	if item.Type == domainlib.Series {
		mediaType = "tv"
	}

	// For series, count episodes in S01 on disk to validate TMDB matches
	var diskS01Episodes int
	if mediaType == "tv" {
		n, cErr := e.countDiskEpisodesForSeason(ctx, string(item.ID), 1)
		if cErr != nil {
			// Skip rather than match blind: a provider match accepted without
			// the disk check is the false positive this guard exists to
			// refuse, and the next pass will retry the series for free.
			if log != nil {
				log.Warn("skip auto-enrich: disk episode count unreadable",
					slog.String("id", string(item.ID)), slog.Any("err", cErr))
			}
			return
		}
		diskS01Episodes = n
	}

	// Search-title precedence for orphans:
	//   1. title override (user's manual correction - strongest signal)
	//   2. originalTitle override (fallback if user only edited that)
	//   3. ScanTitle (stable filesystem-derived, survives bad matches)
	//   4. Title (last resort, possibly polluted by old parser)
	// For matched items the override branch returned above, so this falls
	// through to the historical ScanTitle || Title pair.
	searchTitle := titleOverride
	if searchTitle == "" {
		searchTitle = originalTitleOverride
	}
	if searchTitle == "" {
		searchTitle = item.ScanTitle
	}
	if searchTitle == "" {
		searchTitle = item.Title
	}

	// Year source: item.Catalog.Year is set by prior enrichments. On a
	// forced refresh (admin "Refresh all metadata"), this year may be the
	// one a WRONG previous match wrote - passing it back to TMDB filters
	// the search down to just that wrong result and the exact-title
	// shortcut below only sees 1 candidate (the wrong one) and picks it
	// again. Drop year on force so the search returns the full candidate
	// list and popularity / exact-title ranking can correct the mistake.
	var year *int
	if !force && item.Catalog != nil {
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

	// When a supplementary provider (e.g. OMDb via an IMDB id from the
	// filename/NFO) won the match instead of TMDB, the item ends up with only
	// an IMDB id and whatever English text OMDb returned - and with no TMDB id
	// there are no translations to overlay, so a non-English instance is stuck
	// showing English metadata forever. Map the IMDB id back to a TMDB record
	// and swap in the full localized TMDB detail before persisting.
	if upgraded := e.upgradeToTMDBByIMDB(ctx, primary, mediaType, log); upgraded != nil {
		primary = upgraded
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
			slog.ErrorContext(ctx, "panic in supplementItem", slog.Any("recover", r), slog.String("title", item.Title))
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
		if err := e.itemUpdater.Update(ctx, item); err != nil && log != nil {
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

// upgradeToTMDBByIMDB recovers the canonical TMDB record for a match that a
// non-TMDB provider produced. TMDB's title search can miss (spelling, region,
// alternate titles) on a title that an IMDB-anchored provider resolves exactly;
// TMDB's /find endpoint maps that IMDB id back to a TMDB record, so we re-fetch
// the full TMDB detail (localized base fields, proper artwork/cast) and return
// it as the new canonical result. The IMDB id is the canonical anchor, so this
// only ever corrects a match - it never trades a good one for a worse guess.
//
// Returns nil (caller keeps the original result) when the match already has a
// TMDB id, carries no IMDB id to map, the primary provider can't do an
// external-id lookup, or the lookup / detail fetch yields nothing.
func (e *Enricher) upgradeToTMDBByIMDB(ctx context.Context, result *domainmetadata.MetadataResult, mediaType string, log *slog.Logger) *domainmetadata.MetadataResult {
	if result.ProviderIDs["tmdb"] != "" {
		return nil
	}
	imdbID := result.ProviderIDs["imdb"]
	if imdbID == "" {
		return nil
	}
	prov := e.chain.Primary()
	if prov == nil {
		return nil
	}
	lookup, ok := capabilityFrom[domainmetadata.ExternalIDLookup](prov)
	if !ok {
		return nil
	}
	found, err := lookup.LookupByExternalID(ctx, "imdb", imdbID)
	if err != nil || found == nil {
		return nil
	}
	tmdbID := found.ProviderIDs["tmdb"]
	if tmdbID == "" {
		return nil
	}
	detail, err := prov.GetByProviderID(ctx, tmdbID, mediaType)
	if err != nil || detail == nil {
		if log != nil {
			log.Debug("tmdb-by-imdb detail fetch failed",
				slog.String("imdb", imdbID), slog.String("tmdb", tmdbID), slog.Any("err", err))
		}
		return nil
	}
	// Keep the IMDB id even if the TMDB detail didn't echo it, so the
	// supplementary pass can still resolve ratings/extra IDs by it.
	if detail.ProviderIDs == nil {
		detail.ProviderIDs = map[string]string{}
	}
	if detail.ProviderIDs["imdb"] == "" {
		detail.ProviderIDs["imdb"] = imdbID
	}
	if log != nil {
		log.Info("recovered TMDB match via IMDB id",
			slog.String("imdb", imdbID), slog.String("tmdb", tmdbID))
	}
	return detail
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

// countDiskEpisodesForSeason counts episodes on disk for a given season of a
// series. The count feeds the guard that rejects a provider match whose season
// 1 disagrees with the disk, and 0 legitimately means "no episodes there" -
// which is exactly the value that switches the guard off. A read that FAILED
// must therefore be told apart from a season that is empty, hence the error.
func (e *Enricher) countDiskEpisodesForSeason(ctx context.Context, seriesID string, seasonNum int) (int, error) {
	if e.mediaItems == nil {
		return 0, nil
	}
	seasonType := "season"
	// Explicit limit: no series has this many seasons, and a stated bound reads
	// as a decision where an omitted one reads as an oversight.
	const maxSeasons = 500
	seasons, err := e.mediaItems.List(ctx, domainlib.ListMediaItemsFilter{
		ParentID: &seriesID,
		Type:     &seasonType,
		Limit:    maxSeasons,
	})
	if err != nil {
		return 0, fmt.Errorf("countDiskEpisodesForSeason: list seasons: %w", err)
	}
	for _, s := range seasons {
		if s.IndexNumber != nil && *s.IndexNumber == seasonNum {
			episodeType := "episode"
			seasonID := string(s.ID)
			// Count, not List + len: this wants a number, and a List with no
			// Limit is clamped to 500 rows by the repository, so a very long
			// season (a daily soap, or a flat scan of a long-running show)
			// reported 500 and skewed the episode-count check that validates a
			// provider match. Counting also avoids loading the rows at all.
			n, epErr := e.mediaItems.Count(ctx, domainlib.ListMediaItemsFilter{
				ParentID: &seasonID,
				Type:     &episodeType,
			})
			if epErr != nil {
				return 0, fmt.Errorf("countDiskEpisodesForSeason: count episodes: %w", epErr)
			}
			return n, nil
		}
	}
	return 0, nil
}

// searchAndFetchBest searches a provider and picks the best match, validating
// against disk structure (episode counts) for series.
func (e *Enricher) searchAndFetchBest(ctx context.Context, provider domainmetadata.MetadataProvider, title string, year *int, mediaType string, diskS01Episodes int, log *slog.Logger) *domainmetadata.MetadataResult {
	results, err := provider.Search(ctx, title, year, mediaType)
	if err != nil {
		if log != nil {
			log.Debug("provider search failed",
				slog.String("provider", provider.Name()),
				slog.String("title", title),
				slog.Any("err", err))
		}
		return nil
	}

	// Cascade retry: when the full query returns nothing, progressively trim
	// trailing words (up to 3 retries) to handle filenames with marketing
	// subtitles that poison the search (e.g. "Wanted Choisi Ton Destin" →
	// "Wanted Choisi Ton" → "Wanted Choisi" → "Wanted"). Stop as soon as we
	// get at least one result. Never trim below a single word.
	if len(results) == 0 {
		trimmed := title
		for retry := 0; retry < 3 && len(results) == 0; retry++ {
			idx := strings.LastIndexAny(trimmed, " -")
			if idx <= 0 {
				break // already at one word
			}
			trimmed = strings.TrimRight(trimmed[:idx], " -")
			if trimmed == "" {
				break
			}
			results, err = provider.Search(ctx, trimmed, year, mediaType)
			if err != nil {
				results = nil
			}
			if len(results) > 0 && log != nil {
				log.Debug("cascade query found results",
					slog.String("original", title),
					slog.String("trimmed", trimmed),
					slog.Int("count", len(results)))
			}
		}
	}

	if len(results) == 0 {
		return nil
	}

	// For non-series or when we don't have disk episode info, use simple matching
	if mediaType != "tv" || diskS01Episodes == 0 {
		return e.fetchBestCandidate(ctx, provider, results, title, year, mediaType)
	}

	// Exact-title shortcut BEFORE the series-specific episode-count path.
	// If the search query matches exactly one TMDB result (case-folded +
	// diacritic-stripped), that's the intended series - skip the episode-
	// count heuristic that can be fooled by a documentary with a longer
	// title sharing a word. Fixes e.g. "Billions" (Showtime, 7 seasons)
	// losing to "The Foods that Make Billions" (single doc, shares word).
	qNorm := normalizeForMatch(title)
	if qNorm != "" {
		var exactCandidates []domainmetadata.MetadataResult
		for _, r := range results {
			if normalizeForMatch(r.Title) == qNorm || normalizeForMatch(r.OriginalTitle) == qNorm {
				exactCandidates = append(exactCandidates, r)
			}
		}
		// Several series share an exact title across remakes ("Sherlock
		// Holmes" 1954, 1965, 1984...): the year the user wrote on the show
		// folder picks among them, the way pickBestMatch does for films.
		if len(exactCandidates) > 1 && year != nil {
			var dated []domainmetadata.MetadataResult
			for _, r := range exactCandidates {
				if r.Year != nil && *r.Year == *year {
					dated = append(dated, r)
				}
			}
			if len(dated) == 1 {
				exactCandidates = dated
			}
		}
		if len(exactCandidates) == 1 {
			providerID := exactCandidates[0].ProviderIDs[provider.Name()]
			if providerID != "" {
				if full, fetchErr := provider.GetByProviderID(ctx, providerID, mediaType); fetchErr == nil && full != nil {
					if log != nil {
						log.Debug("series match by exact title",
							slog.String("title", full.Title),
							slog.String("query", title))
					}
					return full
				}
			}
		}
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

	// No candidate matched the disk episode count - use the best fallback
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
// Tuning knobs for seriesFallbackScore. epScoreBase dominates when the
// episode count is close (100 at exact match, halved per episode of
// distance); recency adds recencyWeightPerYear per year after
// recencyPivotYear; popularity is capped at maxPopularityScore so a
// blockbuster can tip a tie but never outvote the episode-count signal.
const (
	epScoreBase          = 100.0
	recencyPivotYear     = 2015
	recencyWeightPerYear = 2.0
	maxPopularityScore   = 50.0
)

func seriesFallbackScore(diskEp, tmdbEp int, year *int, popularity float64) float64 {
	diff := math.Abs(float64(diskEp - tmdbEp))
	epScore := epScoreBase / (1.0 + diff)
	yearScore := 0.0
	if year != nil && *year >= recencyPivotYear {
		yearScore = float64(*year-recencyPivotYear) * recencyWeightPerYear
	}
	popScore := math.Min(popularity, maxPopularityScore)
	return epScore + yearScore + popScore
}

// fetchBestCandidate picks the best title match and fetches full details.
// When no candidate exactly matches the query on Title/OriginalTitle, it
// lazily fetches alternative titles for the top ~5 candidates (capped,
// cached by the provider) and retries the exact-match against the full
// title set. This resolves localised-title mismatches without per-language
// search fan-out (e.g. "Train to Busan" when TMDB returned "Dernier train
// pour Busan" as the primary title).
func (e *Enricher) fetchBestCandidate(ctx context.Context, provider domainmetadata.MetadataProvider, results []domainmetadata.MetadataResult, title string, year *int, mediaType string) *domainmetadata.MetadataResult {
	// Phase 1: cheap exact-match on Title/OriginalTitle - no extra calls.
	qNorm := normalizeForMatch(title)
	if qNorm != "" {
		for _, r := range results {
			if normalizeForMatch(r.Title) == qNorm || normalizeForMatch(r.OriginalTitle) == qNorm {
				goto pickDone // at least one exact match exists; fall through to pickBestMatch
			}
		}
		// Phase 2: no exact match - lazily fetch alt-titles for top-5 candidates.
		if atp, ok := altTitlesCapability(provider); ok {
			capped := results
			if len(capped) > 5 {
				capped = capped[:5]
			}
			enriched := make([]domainmetadata.MetadataResult, len(results))
			copy(enriched, results)
			for i, r := range capped {
				pid := r.ProviderIDs[provider.Name()]
				if pid == "" {
					continue
				}
				alts, err := atp.AlternativeTitles(ctx, pid, mediaType)
				if err != nil || len(alts) == 0 {
					continue
				}
				// Check for an exact hit among the alt titles now, and return
				// early rather than waiting for pickBestMatch - the caller
				// wants the full details for the matched candidate.
				for _, alt := range alts {
					if normalizeForMatch(alt) == qNorm {
						if year != nil && r.Year != nil && *r.Year != *year {
							continue // year mismatch, keep scanning
						}
						// Fetch full details for this candidate.
						full, fetchErr := provider.GetByProviderID(ctx, pid, mediaType)
						if fetchErr != nil {
							// Same reason as the pickDone path below: don't persist
							// the search skeleton on a transient detail-fetch error
							// (non-empty overview + no artwork/cast trips the
							// non-force re-enrich guard and strands the item). Skip
							// the provider so the chain falls through / retries.
							slog.WarnContext(ctx, "fetchBestCandidate: alt-title detail fetch failed, skipping provider",
								slog.String("provider", provider.Name()), slog.Any("err", fetchErr))
							return nil
						}
						if full != nil {
							return full
						}
						return &capped[i]
					}
				}
				enriched[i].AlternativeTitles = alts
			}
			results = enriched
		}
	}

pickDone:
	best := pickBestMatch(results, title, year)
	// pickBestMatch always returns something - its last resort is results[0],
	// which is whatever the provider ranked first even when no candidate has
	// anything to do with the query (see match_confidence.go). Refuse a
	// candidate that names a different work rather than persisting it.
	if !titlePlausible(title, candidateTitles(best)) {
		slog.WarnContext(ctx, "metadata match rejected: title too far from the parsed one",
			slog.String("provider", provider.Name()),
			slog.String("query", title),
			slog.String("candidate", best.Title))
		return nil
	}
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
		// Don't persist the search-level skeleton on a transient detail-fetch
		// failure (rate-limit / network): it carries a non-empty overview but no
		// artwork, cast, trailers or collection, and the non-force re-enrich
		// guard (Overview != "") would then skip the item on every future scan,
		// stranding it in a degraded state until a manual refresh. Returning nil
		// lets the chain fall through to the next provider; if every provider
		// fails the item keeps its prior data and is retried next scan.
		slog.WarnContext(ctx, "fetchBestCandidate: detail fetch failed, skipping provider",
			slog.String("provider", provider.Name()),
			slog.String("title", title),
			slog.Any("err", err))
		return nil
	}
	return full
}

// altTitlesCapability unwraps decorators (retry, cache) to find the
// AlternativeTitlesProvider capability, mirroring capabilityFrom in
// provider_chain.go but without importing the chain from this package.
func altTitlesCapability(p domainmetadata.MetadataProvider) (domainmetadata.AlternativeTitlesProvider, bool) {
	for cur := p; cur != nil; {
		if atp, ok := any(cur).(domainmetadata.AlternativeTitlesProvider); ok {
			return atp, true
		}
		u, ok := cur.(domainmetadata.ProviderUnwrapper)
		if !ok {
			break
		}
		cur = u.Unwrap()
	}
	return nil, false
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
	// Allow 20% tolerance - user may not have all episodes
	tolerance := float64(tmdb) * 0.2
	if tolerance < 1 {
		tolerance = 1
	}
	return float64(diff) <= tolerance
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
// with rate limiting to avoid 429s from external APIs. Items with an
// existing overview + trailers are skipped so re-scans don't flood TMDB.
func (e *Enricher) AutoEnrichAll(ctx context.Context, items []*domainlib.MediaItem, log *slog.Logger) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.AutoEnrichAll")
	defer span.End()
	span.SetAttributes(attribute.Int("items.count", len(items)))
	e.autoEnrichAll(ctx, items, log, false)
}

// ForceAutoEnrichAll bypasses the "already enriched" filter so an admin
// refresh can correct bad past matches across the whole library. Admin
// field overrides are still respected.
func (e *Enricher) ForceAutoEnrichAll(ctx context.Context, items []*domainlib.MediaItem, log *slog.Logger) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ForceAutoEnrichAll")
	defer span.End()
	span.SetAttributes(attribute.Int("items.count", len(items)))
	e.autoEnrichAll(ctx, items, log, true)
}

func (e *Enricher) autoEnrichAll(ctx context.Context, items []*domainlib.MediaItem, log *slog.Logger, force bool) {
	// Batch-load admin overrides ONCE with a single GetByMediaItems call - both
	// the bool set used for the skip decision below and the full records
	// autoEnrichItem needs to harvest orphan title overrides. Previously this
	// preloaded only the bool set, so autoEnrichItem still issued its own
	// GetByMediaItem per item in the hot loop (N+1 on a 7700+ item library).
	overridden := map[string]bool{}
	var overridesByItem map[string][]*domainmetadata.MetadataOverride
	if e.overrides != nil && len(items) > 0 {
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ids = append(ids, string(it.ID))
		}
		m, err := e.overrides.GetByMediaItems(ctx, ids)
		if err != nil {
			// Not fatal - autoEnrichItem re-reads per item and now protects
			// itself when that read fails too. Logged because the silent
			// fallback is an N+1 over the whole library, a cliff that is
			// otherwise invisible.
			if log != nil {
				log.Warn("auto-enrich: batch override preload failed, falling back to per-item reads",
					slog.Int("items", len(ids)), slog.Any("err", err))
			}
		} else {
			overridesByItem = m
			for id, ovs := range m {
				if len(ovs) > 0 {
					overridden[id] = true
				}
			}
		}
	}
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, item := range items {
		// Mirror the autoEnrichItem split: a matched item's overrides are
		// display intent (skip), but an orphan's overrides are the admin's
		// "match-me" signal - let autoEnrichItem run, it will harvest the
		// title/originalTitle overrides into the search query. Without this
		// split, match-orphans → drainOrphans → AutoEnrichAll dropped every
		// orphan-with-override at the bulk gate and the drainer's stall
		// detector exited immediately every single iteration (pré-prod
		// regression: 7700+ orphans frozen because the admin had cleaned
		// their titles via the UI).
		if overridden[string(item.ID)] {
			isOrphan := item.Catalog == nil || len(item.Catalog.ProviderIDs) == 0
			if !isOrphan {
				continue
			}
		}
		// An enriched item stays enriched: without this every boot re-queried the
		// provider for every episode and every trailer-less film (9500 items at
		// 4/s = 40 min of TMDB + search re-indexing per restart, prod 2026-09-10).
		// The admin re-enrich (force) is the way to refresh.
		if !force && item.Overview != "" {
			continue
		}
		if err := e.apiLimiter.Wait(ctx); err != nil {
			return
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(it *domainlib.MediaItem) {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "panic recovered in metadata enricher", "error", r, "stack", string(debug.Stack()))
				}
			}()
			defer wg.Done()
			defer func() { <-sem }()
			e.autoEnrichItem(ctx, it, log, force, overridesByItem)
		}(item)
	}
	wg.Wait()
}

// pickBestMatch trusts TMDB's own popularity+relevance ranking and returns
// the first result that shares at least one word with the query (coherence
// filter). If none do, it falls back to the top TMDB result rather than
// second-guessing TMDB with a local heuristic. When year is provided and a
// shared-word candidate matches it exactly, that candidate wins - remakes/
// reboots with the same title stay disambiguated.
//
// Exact-title shortcut (added because popularity alone mis-ranks generic
// titles): if exactly one candidate has Title or OriginalTitle equal to
// the query after normalisation (case-folded + diacritics stripped), it
// wins before any other scoring. Fixes cases like "Billions" being
// outranked by "The Foods that Make Billions" on word-share + popularity.
// Multiple exact matches fall back to year tiebreaker / popularity order.
func pickBestMatch(results []domainmetadata.MetadataResult, query string, year *int) domainmetadata.MetadataResult {
	if len(results) == 1 {
		return results[0]
	}
	qNorm := normalizeForMatch(query)
	exactIdx := -1
	exactCount := 0
	for i, r := range results {
		if qNorm == "" {
			break
		}
		titleMatch := normalizeForMatch(r.Title) == qNorm || normalizeForMatch(r.OriginalTitle) == qNorm
		if !titleMatch {
			for _, alt := range r.AlternativeTitles {
				if normalizeForMatch(alt) == qNorm {
					titleMatch = true
					break
				}
			}
		}
		if titleMatch {
			if year != nil && r.Year != nil && *r.Year == *year {
				return r
			}
			if exactIdx < 0 {
				exactIdx = i
			}
			exactCount++
		}
	}
	if exactCount == 1 {
		return results[exactIdx]
	}
	if exactCount > 1 {
		return results[exactIdx]
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

// normalizeForMatch lowercases, strips diacritics and collapses punctuation/
// whitespace to enable fuzzy equality between e.g. "Robin Hood" and
// "robin hood" or "Le Flic de Shanghaï" and "Le flic de shanghai".
func normalizeForMatch(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevSpace = false
		case r == ' ', r == '-', r == '_', r == '.', r == ':', r == ';',
			r == ',', r == '\'', r == '"', r == '(', r == ')',
			r == '[', r == ']', r == '{', r == '}':
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
		default:
			// Fold common Latin diacritics to ASCII - keeps the equality
			// robust across TMDB title variants (Shanghaï/Shanghai, Pokémon/
			// Pokemon, etc.) without pulling in golang.org/x/text/unicode.
			if folded, ok := diacriticFold[r]; ok {
				b.WriteRune(folded)
				prevSpace = false
			}
		}
	}
	return strings.TrimSpace(b.String())
}

var diacriticFold = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ī': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ō': 'o', 'ø': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ū': 'u',
	'ý': 'y', 'ÿ': 'y',
	'ñ': 'n', 'ç': 'c', 'ß': 's',
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
		// The em/en dashes (U+2014, U+2013) are INPUT data to strip from
		// external titles, not prose - spelled as escapes so the house style
		// (plain '-' everywhere) holds in the source.
		t := strings.Trim(w, ".,:;!?'\"()[]{}«»\u2014\u2013-")
		if t != "" {
			m[t] = true
		}
	}
	return m
}

// RefreshSeriesChildren re-enriches every season and episode of a series
// from the series' CURRENT provider ids. An admin who re-identifies a series
// changes what its episodes are (the 1954 "Sherlock Holmes" and the 1984 one
// share no episode), so the children must follow; left alone they kept the
// titles and overviews of the previous match. Rate-limited like the bulk
// pass; a child that fails is logged and skipped, the others still land.
func (e *Enricher) RefreshSeriesChildren(ctx context.Context, series *domainlib.MediaItem, log *slog.Logger) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshSeriesChildren")
	defer span.End()
	span.SetAttributes(attribute.String("series.id", string(series.ID)))
	if series.Type != domainlib.Series {
		return nil
	}
	seasonType, episodeType := string(domainlib.Season), string(domainlib.Episode)
	seriesID := string(series.ID)
	seasons, err := e.mediaItems.List(ctx, domainlib.ListMediaItemsFilter{ParentID: &seriesID, Type: &seasonType})
	if err != nil {
		return fmt.Errorf("RefreshSeriesChildren: list seasons: %w", err)
	}
	for _, season := range seasons {
		if err := e.apiLimiter.Wait(ctx); err != nil {
			return err
		}
		e.autoEnrichSeason(ctx, season, log)
		seasonID := string(season.ID)
		episodes, err := e.mediaItems.List(ctx, domainlib.ListMediaItemsFilter{ParentID: &seasonID, Type: &episodeType})
		if err != nil {
			return fmt.Errorf("RefreshSeriesChildren: list episodes of season %s: %w", seasonID, err)
		}
		for _, ep := range episodes {
			if err := e.apiLimiter.Wait(ctx); err != nil {
				return err
			}
			e.autoEnrichEpisode(ctx, ep, log)
		}
	}
	return nil
}
