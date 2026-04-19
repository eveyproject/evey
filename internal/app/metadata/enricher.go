package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"time"

	"github.com/google/uuid"
	domainlib "github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
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
	overrides            domainmetadata.MetadataOverrideRepository
}

type EnricherOption func(*Enricher)

func EnricherWithPersonFetcher(f personDetailFetcher) EnricherOption {
	return func(e *Enricher) { e.personFetcher = f }
}

// EnricherWithOverrides wires in the admin metadata override repo so that
// auto-enrichment skips items whose admin has manually edited any field.
func EnricherWithOverrides(r domainmetadata.MetadataOverrideRepository) EnricherOption {
	return func(e *Enricher) { e.overrides = r }
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
	if err := e.replaceArtwork(ctx, string(item.ID), result.Artwork); err != nil {
		return fmt.Errorf("Enricher.EnrichItem: %w", err)
	}
	if err := e.upsertPeople(ctx, string(item.ID), result.People); err != nil {
		return fmt.Errorf("Enricher.EnrichItem: %w", err)
	}
	e.indexSearch(ctx, item, result.People)
	e.linkCollection(ctx, string(item.ID), result.Collection)
	// Fetch translations for all supported locales
	if item.Catalog != nil {
		if tmdbID := item.Catalog.ProviderIDs["tmdb"]; tmdbID != "" {
			mediaType := "movie"
			if item.Type == "series" || item.Type == "season" || item.Type == "episode" {
				mediaType = "tv"
			}
			e.enrichTranslations(ctx, item, tmdbID, mediaType)
		}
	}
	return nil
}

func applyBasicFields(item *domainlib.MediaItem, result *domainmetadata.MetadataResult) {
	item.Title = result.Title
	item.OriginalTitle = result.OriginalTitle
	item.Overview = result.Overview
	item.Tagline = result.Tagline

	if item.Catalog == nil {
		item.Catalog = &domainlib.MediaCatalog{}
	}
	item.Catalog.Genres = result.Genres
	item.Catalog.Studios = result.Studios
	item.Catalog.PremiereDate = result.PremiereDate
	item.Catalog.Year = result.Year
	if item.Catalog.ProviderIDs == nil {
		item.Catalog.ProviderIDs = make(map[string]string)
	}
	maps.Copy(item.Catalog.ProviderIDs, result.ProviderIDs)

	if item.Classification == nil {
		item.Classification = &domainlib.MediaClassification{}
	}
	item.Classification.CommunityRating = result.CommunityRating
	item.Classification.OfficialRating = result.OfficialRating
	if len(result.CertificationsByCountry) > 0 {
		item.Classification.CertificationsByCountry = maps.Clone(result.CertificationsByCountry)
	}
	if item.Classification.ProviderRatings == nil {
		item.Classification.ProviderRatings = make(map[string]float64)
	}
	maps.Copy(item.Classification.ProviderRatings, result.Ratings)

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
}

func (e *Enricher) replaceArtwork(ctx context.Context, mediaItemID string, artwork []domainmetadata.ArtworkResult) error {
	if err := e.artwork.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		return fmt.Errorf("delete old artwork: %w", err)
	}
	if len(artwork) == 0 {
		return nil
	}
	records := make([]*domainmetadata.ArtworkRecord, 0, len(artwork))
	for i, art := range artwork {
		if rec := buildArtworkRecord(mediaItemID, i, art); rec != nil {
			records = append(records, rec)
		}
	}
	if len(records) == 0 {
		return nil
	}
	if err := e.artwork.CreateBatch(ctx, records); err != nil {
		return fmt.Errorf("create artwork batch: %w", err)
	}
	return nil
}

func buildArtworkRecord(mediaItemID string, sortOrder int, art domainmetadata.ArtworkResult) *domainmetadata.ArtworkRecord {
	provider := art.Provider
	if provider == "" {
		provider = "unknown"
	}
	opts := []domainmetadata.ArtworkRecordOption{
		domainmetadata.WithArtworkSource(art.URL),
		domainmetadata.WithArtworkSortOrder(sortOrder),
	}
	if art.Language != "" {
		opts = append(opts, domainmetadata.WithArtworkLanguage(art.Language))
	}
	if art.Width > 0 && art.Height > 0 {
		opts = append(opts, domainmetadata.WithArtworkDimensions(art.Width, art.Height))
	}
	rec, err := domainmetadata.NewArtworkRecord(uuid.New().String(), shared.MediaItemID(mediaItemID), art.Type, provider, opts...)
	if err != nil {
		return nil
	}
	return rec
}

func (e *Enricher) upsertPeople(ctx context.Context, mediaItemID string, people []domainmetadata.PersonResult) error {
	if err := e.mediaPersons.DeleteByMediaItemID(ctx, shared.MediaItemID(mediaItemID)); err != nil {
		return fmt.Errorf("delete old media_people: %w", err)
	}
	seenPersons := make(map[string]bool)
	batch := make([]*domainlib.MediaPerson, 0, len(people))
	sortIdx := 0
	for _, p := range people {
		if seenPersons[p.Name] {
			continue
		}
		seenPersons[p.Name] = true

		person, err := e.findOrCreatePerson(ctx, p)
		if err != nil {
			return fmt.Errorf("Enricher.upsertPeople: %w", err)
		}

		var charName *string
		if p.Character != "" {
			c := p.Character
			charName = &c
		}
		batch = append(batch, &domainlib.MediaPerson{
			MediaItemID:   shared.MediaItemID(mediaItemID),
			PersonID:      person.ID,
			RoleType:      roleType(p.Role),
			CharacterName: charName,
			SortOrder:     sortIdx,
		})
		sortIdx++
	}
	if err := e.mediaPersons.CreateBatch(ctx, batch); err != nil {
		slog.Warn("batch create media persons failed", slog.String("mediaItemID", mediaItemID), slog.Any("err", err))
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

func (e *Enricher) EnrichTranslationsForItem(ctx context.Context, item *domainlib.MediaItem, providerID, mediaType string) {
	e.enrichTranslations(ctx, item, providerID, mediaType)
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
		slog.Warn("fetch translations", slog.String("item", string(item.ID)), slog.Any("err", err))
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
