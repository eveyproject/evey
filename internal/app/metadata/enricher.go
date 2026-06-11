package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"time"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"golang.org/x/time/rate"
)

// CollectionLinker upserts provider-sourced collections and links media items.
type CollectionLinker interface {
	Upsert(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*domainlib.Collection, error)
	LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
}

// ArtworkPostSaveHook is called for each artwork record immediately after it is
// persisted during scan enrichment. Used to trigger auto-download when enabled.
type ArtworkPostSaveHook func(ctx context.Context, rec *domainmetadata.ArtworkRecord)

type Enricher struct {
	artwork             domainmetadata.ArtworkRepository
	persons             domainlib.PersonRepository
	mediaPersons        domainlib.MediaPersonRepository
	mediaItems          domainlib.MediaItemRepository
	search              domainlib.SearchIndex
	chain               *ProviderChain
	collections         CollectionLinker
	apiLimiter          *rate.Limiter
	mediaTranslations   domainlib.MediaItemTranslationRepository
	personTranslations  domainlib.PersonTranslationRepository
	translationLocales  []string
	personFetcher       personDetailFetcher
	overrides           domainmetadata.MetadataOverrideRepository
	artworkPostSaveHook ArtworkPostSaveHook
	// uow makes EnrichItem's three writes (item update, artwork replace,
	// people upsert) one transaction. nil (tests / no wiring) keeps the
	// historical per-write auto-commit behaviour.
	uow shared.TransactionalUnit
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

// EnricherWithArtworkPostSaveHook registers a callback fired after each artwork
// record batch is persisted. CommandService uses this to trigger auto-download.
func EnricherWithArtworkPostSaveHook(hook ArtworkPostSaveHook) EnricherOption {
	return func(e *Enricher) { e.artworkPostSaveHook = hook }
}

// EnricherWithUnitOfWork makes EnrichItem's persistence steps atomic: the item
// update, the artwork delete+insert and the cast/crew delete+insert commit or
// roll back together. Without it a failure between the steps left the item
// with a fresh title but NO artwork and NO cast — and since updateMediaFile
// only republishes MediaItemAdded when Overview is empty (now non-empty), a
// re-scan never re-enriched it; only a manual "Refresh metadata" repaired it.
func EnricherWithUnitOfWork(uow shared.TransactionalUnit) EnricherOption {
	return func(e *Enricher) { e.uow = uow }
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
		artwork:      artwork,
		persons:      persons,
		mediaPersons: mediaPersons,
		mediaItems:   mediaItems,
		search:       search,
		chain:        NewProviderChain(primary, supplementary...),
		collections:  collections,
		apiLimiter:   rate.NewLimiter(rate.Every(250*time.Millisecond), 1),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// EnrichItem applies a MetadataResult to a media item: updates the item,
// replaces artwork records, and upserts people/cast — atomically when a unit
// of work is wired (EnricherWithUnitOfWork). Side effects that must not run
// inside the transaction are HOISTED after the commit:
//   - artworkPostSaveHook (auto-download trigger): firing it mid-tx would
//     start a download for artwork rows a later failure rolls back — a
//     phantom download for records that never existed;
//   - person bio fetches: each one is a rate-limited TMDB round-trip (250 ms
//     apart, casts run to 50 people) — holding a row-locking transaction
//     open across them would stall every concurrent writer of those rows.
func (e *Enricher) EnrichItem(ctx context.Context, item *domainlib.MediaItem, result *domainmetadata.MetadataResult) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.EnrichItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(item.ID)))
	applyBasicFields(item, result)

	var artworkRecords []*domainmetadata.ArtworkRecord
	var bioPending []*domainlib.Person
	apply := func(ctx context.Context) error {
		if err := e.mediaItems.Update(ctx, item); err != nil {
			return fmt.Errorf("update media item: %w", err)
		}
		records, err := e.replaceArtwork(ctx, string(item.ID), result.Artwork)
		if err != nil {
			return err
		}
		artworkRecords = records
		pending, err := e.upsertPeople(ctx, string(item.ID), result.People)
		if err != nil {
			return err
		}
		bioPending = pending
		return nil
	}
	var txErr error
	if e.uow != nil {
		txErr = e.uow.Do(ctx, apply)
	} else {
		txErr = apply(ctx)
	}
	if txErr != nil {
		span.RecordError(txErr)
		span.SetStatus(otelcodes.Error, txErr.Error())
		return fmt.Errorf("Enricher.EnrichItem: %w", txErr)
	}

	// Post-commit side effects — the rows they reference are now durable.
	if e.artworkPostSaveHook != nil {
		for _, rec := range artworkRecords {
			e.artworkPostSaveHook(ctx, rec)
		}
	}
	for _, person := range bioPending {
		e.fetchPersonBio(ctx, person)
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

// replaceArtwork swaps the item's artwork rows and returns the freshly
// persisted records. It does NOT fire artworkPostSaveHook — that is the
// caller's post-commit duty (firing inside the transaction would trigger an
// auto-download for rows a later failure rolls back).
func (e *Enricher) replaceArtwork(ctx context.Context, mediaItemID string, artwork []domainmetadata.ArtworkResult) ([]*domainmetadata.ArtworkRecord, error) {
	if err := e.artwork.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		return nil, fmt.Errorf("delete old artwork: %w", err)
	}
	if len(artwork) == 0 {
		return nil, nil
	}
	records := make([]*domainmetadata.ArtworkRecord, 0, len(artwork))
	for i, art := range artwork {
		if rec := buildArtworkRecord(mediaItemID, i, art); rec != nil {
			records = append(records, rec)
		}
	}
	if len(records) == 0 {
		return nil, nil
	}
	if err := e.artwork.CreateBatch(ctx, records); err != nil {
		return nil, fmt.Errorf("create artwork batch: %w", err)
	}
	return records, nil
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

// upsertPeople replaces the item's cast/crew links. It returns the persons
// whose biography fetch is PENDING: each fetch is a rate-limited TMDB
// round-trip, so the caller runs them after the transaction commits instead
// of holding row locks across external HTTP calls.
func (e *Enricher) upsertPeople(ctx context.Context, mediaItemID string, people []domainmetadata.PersonResult) ([]*domainlib.Person, error) {
	if err := e.mediaPersons.DeleteByMediaItemID(ctx, shared.MediaItemID(mediaItemID)); err != nil {
		return nil, fmt.Errorf("delete old media_people: %w", err)
	}
	// Preload every candidate person in one query so the findOrCreate
	// loop doesn't turn a 50-person cast into 50 GetByName round-trips.
	uniqueNames := make([]string, 0, len(people))
	seenPersons := make(map[string]bool)
	for _, p := range people {
		if seenPersons[p.Name] {
			continue
		}
		seenPersons[p.Name] = true
		uniqueNames = append(uniqueNames, p.Name)
	}
	known, err := e.persons.GetByNamesBatch(ctx, uniqueNames)
	if err != nil {
		slog.Warn("preload persons failed, falling back to per-row lookup", slog.Any("err", err))
		known = map[string]*domainlib.Person{}
	}

	batch := make([]*domainlib.MediaPerson, 0, len(people))
	seen := make(map[string]bool, len(people))
	var bioPending []*domainlib.Person
	sortIdx := 0
	for _, p := range people {
		if seen[p.Name] {
			continue
		}
		seen[p.Name] = true

		person, needsBio, err := e.findOrCreatePerson(ctx, p, known[p.Name])
		if err != nil {
			return nil, fmt.Errorf("Enricher.upsertPeople: %w", err)
		}
		if needsBio {
			bioPending = append(bioPending, person)
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
		// Abort like the artwork/item-update siblings instead of warn-and-continue:
		// findOrCreatePerson already committed the person rows above, so dropping
		// this batch silently leaves orphan persons with no cast/crew links AND
		// reports the per-item "Refresh metadata" command as a success. Surface it
		// so the caller can retry (the writes are idempotent on re-run).
		return nil, fmt.Errorf("upsertPeople: create media_person batch: %w", err)
	}
	return bioPending, nil
}

// findOrCreatePerson takes the preloaded person (may be nil when absent
// from DB) and either enriches the existing row with new provider IDs/
// thumbnails, or creates a fresh row. needsBio reports that the row has a
// TMDB ID but no biography yet — the CALLER fetches it after the enrichment
// transaction commits (a TMDB round-trip must not run under row locks).
func (e *Enricher) findOrCreatePerson(ctx context.Context, p domainmetadata.PersonResult, preloaded *domainlib.Person) (person *domainlib.Person, needsBio bool, err error) {
	if preloaded != nil {
		person = e.refreshKnownPerson(ctx, p, preloaded)
		return person, person.Biography == "" && person.ProviderIDs["tmdb"] != "", nil
	}
	person, err = e.persons.GetByName(ctx, p.Name)
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
		return person, person.Biography == "" && person.ProviderIDs["tmdb"] != "", nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, false, fmt.Errorf("get person by name: %w", err)
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
		return nil, false, fmt.Errorf("create person: %w", err)
	}
	return person, person.ProviderIDs["tmdb"] != "", nil
}

// refreshKnownPerson mirrors the "already exists" branch of findOrCreatePerson
// for a person already loaded via GetByNamesBatch: fill missing provider IDs
// and thumbnail if the TMDB payload has them, and persist when anything
// changed. Bio fetching is the caller's post-commit duty.
func (e *Enricher) refreshKnownPerson(ctx context.Context, p domainmetadata.PersonResult, person *domainlib.Person) *domainlib.Person {
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
	return person
}

// fetchPersonBio fetches biography from the primary provider and updates the person record.
func (e *Enricher) fetchPersonBio(ctx context.Context, person *domainlib.Person) {
	tp, ok := e.chain.TranslationProvider()
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
	if fetcher, ok2 := capabilityFrom[personDetailFetcher](e.chain.Primary()); ok2 {
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
	ctx, span := tracing.StartSpan(ctx, "app.metadata.EnrichTranslationsForItem")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", string(item.ID)),
		attribute.String("provider.id", providerID),
		attribute.String("media.type", mediaType),
	)
	e.enrichTranslations(ctx, item, providerID, mediaType)
}

func (e *Enricher) enrichTranslations(ctx context.Context, item *domainlib.MediaItem, providerID, mediaType string) {
	if e.mediaTranslations == nil || len(e.translationLocales) == 0 {
		return
	}
	tp, ok := e.chain.TranslationProvider()
	if !ok {
		return
	}
	results, err := tp.FetchTranslations(ctx, providerID, mediaType, e.translationLocales)
	if err != nil {
		slog.Warn("fetch translations", slog.String("item", string(item.ID)), slog.Any("err", err))
		return
	}
	if len(results) == 0 {
		return
	}
	batch := make([]*domainlib.MediaItemTranslation, 0, len(results))
	for locale, result := range results {
		batch = append(batch, &domainlib.MediaItemTranslation{
			MediaItemID: item.ID,
			Locale:      locale,
			Title:       result.Title,
			Overview:    result.Overview,
			Tagline:     result.Tagline,
		})
	}
	if err := e.mediaTranslations.UpsertBatch(ctx, batch); err != nil {
		slog.Warn("upsert media translations batch", slog.String("item", string(item.ID)), slog.Any("err", err))
	}
}

func (e *Enricher) EnrichPersonTranslations(ctx context.Context, person *domainlib.Person, tmdbID int) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.EnrichPersonTranslations")
	defer span.End()
	span.SetAttributes(
		attribute.String("person.id", person.ID),
		attribute.Int("tmdb.id", tmdbID),
	)
	if e.personTranslations == nil || len(e.translationLocales) == 0 {
		return
	}
	tp, ok := e.chain.TranslationProvider()
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
