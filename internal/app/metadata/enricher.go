package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	domainlib "github.com/gravity-zero/evey/internal/domain/library"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"golang.org/x/time/rate"
)

// CollectionLinker upserts provider-sourced collections and links media items.
type CollectionLinker interface {
	Upsert(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*domainlib.Collection, error)
	LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
}

type Enricher struct {
	artwork              domainmetadata.ArtworkRepository
	persons              domainlib.PersonRepository
	mediaPersons         domainlib.MediaPersonRepository
	mediaItems           domainlib.MediaItemRepository
	search               domainlib.SearchIndex
	primary              domainmetadata.MetadataProvider
	supplementary        []domainmetadata.MetadataProvider
	collections          CollectionLinker
	apiLimiter           *rate.Limiter
	mediaTranslations    domainlib.MediaItemTranslationRepository
	personTranslations   domainlib.PersonTranslationRepository
	translationLocales   []string
	personFetcher        personDetailFetcher
}

type EnricherOption func(*Enricher)

func EnricherWithPersonFetcher(f personDetailFetcher) EnricherOption {
	return func(e *Enricher) { e.personFetcher = f }
}

func EnricherWithTranslations(
	mediaTranslations domainlib.MediaItemTranslationRepository,
	personTranslations domainlib.PersonTranslationRepository,
	locales []string,
) EnricherOption {
	return func(e *Enricher) {
		e.mediaTranslations = mediaTranslations
		e.personTranslations = personTranslations
		e.translationLocales = locales
	}
}

func NewEnricher(
	artwork domainmetadata.ArtworkRepository,
	persons domainlib.PersonRepository,
	mediaPersons domainlib.MediaPersonRepository,
	mediaItems domainlib.MediaItemRepository,
	search domainlib.SearchIndex,
	primary domainmetadata.MetadataProvider,
	supplementary []domainmetadata.MetadataProvider,
	collections CollectionLinker,
	opts ...EnricherOption,
) *Enricher {
	e := &Enricher{
		artwork:       artwork,
		persons:       persons,
		mediaPersons:  mediaPersons,
		mediaItems:    mediaItems,
		search:        search,
		primary:       primary,
		supplementary: supplementary,
		collections:   collections,
		apiLimiter:    rate.NewLimiter(rate.Every(250*time.Millisecond), 1),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// EnrichItem applies a MetadataResult to a media item: updates the item,
// replaces artwork records, and upserts people/cast.
func (e *Enricher) EnrichItem(ctx context.Context, item *domainlib.MediaItem, result *domainmetadata.MetadataResult) error {
	applyBasicFields(item, result)

	if err := e.mediaItems.Update(ctx, item); err != nil {
		return fmt.Errorf("update media item: %w", err)
	}
	if err := e.replaceArtwork(ctx, item.ID, result.Artwork); err != nil {
		return err
	}
	if err := e.upsertPeople(ctx, item.ID, result.People); err != nil {
		return err
	}
	e.indexSearch(ctx, item, result.People)
	e.linkCollection(ctx, item.ID, result.Collection)
	return nil
}

func applyBasicFields(item *domainlib.MediaItem, result *domainmetadata.MetadataResult) {
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
	item.Trailers = make([]domainlib.Trailer, 0, len(result.Trailers))
	for _, t := range result.Trailers {
		item.Trailers = append(item.Trailers, domainlib.Trailer{
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
}

func (e *Enricher) replaceArtwork(ctx context.Context, mediaItemID string, artwork []domainmetadata.ArtworkResult) error {
	if err := e.artwork.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		return fmt.Errorf("delete old artwork: %w", err)
	}
	if len(artwork) == 0 {
		return nil
	}
	records := make([]*domainmetadata.ArtworkRecord, len(artwork))
	for i, art := range artwork {
		records[i] = buildArtworkRecord(mediaItemID, i, art)
	}
	if err := e.artwork.CreateBatch(ctx, records); err != nil {
		return fmt.Errorf("create artwork batch: %w", err)
	}
	return nil
}

func buildArtworkRecord(mediaItemID string, sortOrder int, art domainmetadata.ArtworkResult) *domainmetadata.ArtworkRecord {
	src := art.URL
	lang := art.Language
	w, h := art.Width, art.Height
	provider := art.Provider
	if provider == "" {
		provider = "unknown"
	}
	rec := &domainmetadata.ArtworkRecord{
		ID:          uuid.New().String(),
		MediaItemID: mediaItemID,
		Type:        art.Type,
		Provider:    provider,
		SourceURL:   &src,
		SortOrder:   sortOrder,
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
	return rec
}

func (e *Enricher) upsertPeople(ctx context.Context, mediaItemID string, people []domainmetadata.PersonResult) error {
	if err := e.mediaPersons.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		return fmt.Errorf("delete old media_people: %w", err)
	}
	seenPersons := make(map[string]bool)
	sortIdx := 0
	for _, p := range people {
		if seenPersons[p.Name] {
			continue
		}
		seenPersons[p.Name] = true

		person, err := e.findOrCreatePerson(ctx, p)
		if err != nil {
			return err
		}

		var charName *string
		if p.Character != "" {
			c := p.Character
			charName = &c
		}
		mp := &domainlib.MediaPerson{
			MediaItemID:   mediaItemID,
			PersonID:      person.ID,
			RoleType:      roleType(p.Role),
			CharacterName: charName,
			SortOrder:     sortIdx,
		}
		sortIdx++
		if createErr := e.mediaPersons.Create(ctx, mp); createErr != nil {
			slog.Warn("create media person failed", slog.String("personID", person.ID), slog.String("mediaItemID", mediaItemID), slog.Any("err", createErr))
		}
	}
	return nil
}

func (e *Enricher) findOrCreatePerson(ctx context.Context, p domainmetadata.PersonResult) (*domainlib.Person, error) {
	person, err := e.persons.GetByName(ctx, p.Name)
	if err == nil {
		// Update provider ID + thumb if missing
		updated := false
		if p.ProviderID != "" && person.ProviderIDs["tmdb"] == "" {
			if person.ProviderIDs == nil {
				person.ProviderIDs = map[string]string{}
			}
			person.ProviderIDs["tmdb"] = p.ProviderID
			updated = true
		}
		if p.ThumbURL != "" && person.ThumbPath == nil {
			person.ThumbPath = &p.ThumbURL
			updated = true
		}
		if updated {
			if err := e.persons.Update(ctx, person); err != nil {
				slog.Warn("update person failed", slog.String("personID", person.ID), slog.Any("err", err))
			}
		}
		// Fetch bio if missing and we have a TMDB ID
		if person.Biography == "" && person.ProviderIDs["tmdb"] != "" {
			e.fetchPersonBio(ctx, person)
		}
		return person, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("get person by name: %w", err)
	}
	thumbPath := p.ThumbURL
	var thumbPtr *string
	if thumbPath != "" {
		thumbPtr = &thumbPath
	}
	providerIDs := map[string]string{}
	if p.ProviderID != "" {
		providerIDs["tmdb"] = p.ProviderID
	}
	person = &domainlib.Person{
		ID:          uuid.New().String(),
		Name:        p.Name,
		ProviderIDs: providerIDs,
		ThumbPath:   thumbPtr,
	}
	if err := e.persons.Create(ctx, person); err != nil {
		return nil, fmt.Errorf("create person: %w", err)
	}
	// Fetch bio for newly created person
	if person.ProviderIDs["tmdb"] != "" {
		e.fetchPersonBio(ctx, person)
	}
	return person, nil
}

// fetchPersonBio fetches biography from the primary provider and updates the person record.
func (e *Enricher) fetchPersonBio(ctx context.Context, person *domainlib.Person) {
	tp, ok := e.primary.(domainmetadata.TranslationProvider)
	if !ok {
		return
	}
	tmdbID := person.ProviderIDs["tmdb"]
	if tmdbID == "" {
		return
	}
	// Fetch bio in default language
	bios, err := tp.FetchPersonTranslations(ctx, tmdbID, []string{""})
	if err != nil {
		return
	}
	// FetchPersonTranslations with empty locale returns nothing useful — use the person detail endpoint instead
	// Fall back to fetching via the TMDB person detail
	if fetcher, ok2 := e.primary.(personDetailFetcher); ok2 {
		details, detErr := fetcher.GetPersonDetails(ctx, person.Name)
		if detErr == nil && details != nil && details.Biography != "" {
			person.Biography = details.Biography
			if details.Birthday != "" {
				person.Birthday = details.Birthday
			}
			if details.Deathday != "" {
				person.Deathday = details.Deathday
			}
			if details.PlaceOfBirth != "" {
				person.PlaceOfBirth = details.PlaceOfBirth
			}
			if details.KnownFor != "" {
				person.KnownFor = details.KnownFor
			}
			if err := e.persons.Update(ctx, person); err != nil {
				slog.Warn("update person bio failed", slog.String("personID", person.ID), slog.Any("err", err))
			}
		}
	}
	_ = bios // not used directly — translations fetched separately
}

// personDetailFetcher is an optional capability for providers that can fetch person details.
type personDetailFetcher interface {
	GetPersonDetails(ctx context.Context, name string) (*domainlib.PersonDetailsResult, error)
}

func (e *Enricher) indexSearch(ctx context.Context, item *domainlib.MediaItem, people []domainmetadata.PersonResult) {
	if e.search == nil {
		return
	}
	names := make([]string, 0, len(people))
	for _, p := range people {
		names = append(names, p.Name)
	}
	if err := e.search.IndexMediaItem(ctx, item, names); err != nil {
		slog.Warn("index media item", slog.Any("err", err))
	}
}

func (e *Enricher) linkCollection(ctx context.Context, mediaItemID string, ref *domainmetadata.CollectionRef) {
	if e.collections == nil || ref == nil {
		return
	}
	col, err := e.collections.Upsert(ctx, ref.TmdbID, ref.Name, "", ref.PosterURL, ref.BackdropURL)
	if err != nil {
		slog.Warn("upsert collection", slog.Int("tmdb_id", ref.TmdbID), slog.Any("err", err))
		return
	}
	if col == nil {
		return
	}
	if err := e.collections.LinkMediaItem(ctx, col.ID, mediaItemID, 0); err != nil {
		slog.Warn("link media item to collection", slog.String("collection_id", col.ID), slog.String("media_item_id", mediaItemID), slog.Any("err", err))
	}
}

func (e *Enricher) enrichTranslations(ctx context.Context, item *domainlib.MediaItem, providerID, mediaType string) {
	if e.mediaTranslations == nil || len(e.translationLocales) == 0 {
		return
	}
	tp, ok := e.primary.(domainmetadata.TranslationProvider)
	if !ok {
		return
	}
	results, err := tp.FetchTranslations(ctx, providerID, mediaType, e.translationLocales)
	if err != nil {
		slog.Warn("fetch translations", slog.String("item", item.ID), slog.Any("err", err))
		return
	}
	for locale, result := range results {
		if err := e.mediaTranslations.Upsert(ctx, &domainlib.MediaItemTranslation{
			MediaItemID: item.ID,
			Locale:      locale,
			Title:       result.Title,
			Overview:    result.Overview,
			Tagline:     result.Tagline,
		}); err != nil {
			slog.Warn("upsert media translation", slog.String("locale", locale), slog.Any("err", err))
		}
	}
}

func (e *Enricher) EnrichPersonTranslations(ctx context.Context, person *domainlib.Person, tmdbID int) {
	if e.personTranslations == nil || len(e.translationLocales) == 0 {
		return
	}
	tp, ok := e.primary.(domainmetadata.TranslationProvider)
	if !ok {
		return
	}
	bios, err := tp.FetchPersonTranslations(ctx, strconv.Itoa(tmdbID), e.translationLocales)
	if err != nil {
		slog.Warn("fetch person translations", slog.String("person", person.ID), slog.Any("err", err))
		return
	}
	for locale, bio := range bios {
		if err := e.personTranslations.Upsert(ctx, &domainlib.PersonTranslation{
			PersonID:  person.ID,
			Locale:    locale,
			Biography: bio,
		}); err != nil {
			slog.Warn("upsert person translation", slog.String("locale", locale), slog.Any("err", err))
		}
	}
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

// EnrichAllPeopleBios fetches bios for all people missing biography data
// using the person detail fetcher (TMDB search by name → details).
func (e *Enricher) EnrichAllPeopleBios(ctx context.Context, log *slog.Logger) (int, error) {
	fetcher := e.personFetcher
	if fetcher == nil {
		return 0, fmt.Errorf("person fetcher not configured")
	}
	all, err := e.persons.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list people: %w", err)
	}
	enriched := 0
	for _, person := range all {
		if ctx.Err() != nil {
			break
		}
		if person.Biography != "" {
			continue
		}
		details, detErr := fetcher.GetPersonDetails(ctx, person.Name)
		if detErr != nil || details == nil || details.Biography == "" {
			if log != nil {
				log.Debug("no bio found", slog.String("name", person.Name))
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		person.Biography = details.Biography
		if details.Birthday != "" {
			person.Birthday = details.Birthday
		}
		if details.Deathday != "" {
			person.Deathday = details.Deathday
		}
		if details.PlaceOfBirth != "" {
			person.PlaceOfBirth = details.PlaceOfBirth
		}
		if details.KnownFor != "" {
			person.KnownFor = details.KnownFor
		}
		if details.ThumbURL != "" && person.ThumbPath == nil {
			person.ThumbPath = &details.ThumbURL
		}
		if updateErr := e.persons.Update(ctx, person); updateErr != nil {
			slog.Warn("update person bio", slog.String("name", person.Name), slog.Any("err", updateErr))
		} else {
			enriched++
			if log != nil {
				log.Debug("enriched person bio", slog.String("name", person.Name))
			}
		}
		// Rate limit: ~2 req/s to avoid TMDB throttling
		time.Sleep(500 * time.Millisecond)
	}
	return enriched, nil
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
