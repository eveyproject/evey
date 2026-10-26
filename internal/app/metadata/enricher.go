package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"time"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"golang.org/x/time/rate"
)

// bioRecheckTTL bounds how often a person the provider had NO biography for
// is asked again. Without it, a bio-less person is re-fetched on every
// re-enrichment pass (daily), forever - one provider round-trip per
// bio-less person per day for the whole library.
const bioRecheckTTL = 30 * 24 * time.Hour

// CollectionLinker upserts provider-sourced collections and links media items.
type CollectionLinker interface {
	Upsert(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*domainlib.Collection, error)
	LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
}

// UnlinkedMovieLister pages the films no collection holds yet - the read side
// of RelinkCollections, whose writes go through CollectionLinker.
type UnlinkedMovieLister interface {
	ListMoviesWithoutCollection(ctx context.Context, afterID string, limit int) ([]domainlib.UnlinkedMovie, error)
}

// MediaItemUpdater is the one write metadata makes to a media item: saving the
// enriched row. Reads go through domainlib.MediaItemQueryRepo (ADR-0009).
type MediaItemUpdater interface {
	Update(ctx context.Context, item *domainlib.MediaItem) error
}

// ArtworkPostSaveHook is called for each artwork record immediately after it is
// persisted during scan enrichment. Used to trigger auto-download when enabled.
type ArtworkPostSaveHook func(ctx context.Context, rec *domainmetadata.ArtworkRecord)

type Enricher struct {
	// artwork is split into its ADR-0009 command/query halves; both fields are
	// populated from the same concrete repository (see NewEnricher), the split
	// only narrows what each call site in this file can reach.
	artworkCmd          domainmetadata.ArtworkCommandRepo
	artworkQuery        domainmetadata.ArtworkQueryRepo
	persons             domainlib.PersonRepository
	mediaPersons        domainlib.MediaPersonRepository
	mediaItems          domainlib.MediaItemQueryRepo
	itemUpdater         MediaItemUpdater
	search              domainlib.SearchIndex
	chain               *ProviderChain
	collections         CollectionLinker
	unlinkedMovies      UnlinkedMovieLister
	apiLimiter          *rate.Limiter
	mediaTranslations   domainlib.MediaItemTranslationRepository
	personTranslations  domainlib.PersonTranslationRepository
	translationLocales  []string
	personFetcher       personDetailFetcher
	overrides           domainmetadata.MetadataOverrideRepository
	pluginSource        domainmetadata.PluginMetadataSource
	pluginTexts         domainlib.PluginTextRepository
	artworkPostSaveHook ArtworkPostSaveHook
	// uow makes EnrichItem's three writes (item update, artwork replace,
	// people upsert) one transaction. nil (tests / no wiring) keeps the
	// historical per-write auto-commit behaviour.
	uow shared.TransactionalUnit
	// now is the injectable clock behind bioRecheckTTL; tests override it via
	// EnricherWithClock, production keeps the default set in NewEnricher.
	now func() time.Time
}

type EnricherOption func(*Enricher)

// EnricherWithPluginTexts asks the metadata plugins about every movie and
// series it enriches and keeps their texts beside TMDB's (see
// enrichPluginTexts).
func EnricherWithPluginTexts(source domainmetadata.PluginMetadataSource, texts domainlib.PluginTextRepository) EnricherOption {
	return func(e *Enricher) {
		e.pluginSource = source
		e.pluginTexts = texts
	}
}

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

// SetArtworkPostSaveHook wires the post-save hook after construction. Needed
// because the CommandService that owns the hook (maybeAutoDownload) is itself
// built WITH the enricher - the back-reference can only be set once both exist,
// so it cannot go through the NewEnricher option. Wire-time only (no concurrent
// enrichment runs yet), so no locking.
func (e *Enricher) SetArtworkPostSaveHook(hook ArtworkPostSaveHook) {
	e.artworkPostSaveHook = hook
}

// EnricherWithUnitOfWork makes EnrichItem's persistence steps atomic: the item
// update, the artwork delete+insert and the cast/crew delete+insert commit or
// roll back together. Without it a failure between the steps left the item
// with a fresh title but NO artwork and NO cast - and since updateMediaFile
// only republishes MediaItemAdded when Overview is empty (now non-empty), a
// re-scan never re-enriched it; only a manual "Refresh metadata" repaired it.
func EnricherWithUnitOfWork(uow shared.TransactionalUnit) EnricherOption {
	return func(e *Enricher) { e.uow = uow }
}

// EnricherWithClock overrides the clock behind bioRecheckTTL. Tests only;
// production keeps the time.Now default set in NewEnricher.
func EnricherWithClock(now func() time.Time) EnricherOption {
	return func(e *Enricher) { e.now = now }
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
	mediaItems domainlib.MediaItemQueryRepo,
	itemUpdater MediaItemUpdater,
	search domainlib.SearchIndex,
	primary domainmetadata.MetadataProvider,
	supplementary []domainmetadata.MetadataProvider,
	collections CollectionLinker,
	unlinkedMovies UnlinkedMovieLister,
	opts ...EnricherOption,
) *Enricher {
	e := &Enricher{
		artworkCmd:     artwork,
		artworkQuery:   artwork,
		persons:        persons,
		mediaPersons:   mediaPersons,
		mediaItems:     mediaItems,
		itemUpdater:    itemUpdater,
		search:         search,
		chain:          NewProviderChain(primary, supplementary...),
		collections:    collections,
		unlinkedMovies: unlinkedMovies,
		apiLimiter:     rate.NewLimiter(rate.Every(250*time.Millisecond), 1),
		now:            time.Now,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// EnrichItem applies a MetadataResult to a media item: updates the item,
// replaces artwork records, and upserts people/cast - atomically when a unit
// of work is wired (EnricherWithUnitOfWork). Side effects that must not run
// inside the transaction are HOISTED after the commit:
//   - artworkPostSaveHook (auto-download trigger): firing it mid-tx would
//     start a download for artwork rows a later failure rolls back - a
//     phantom download for records that never existed;
//   - person bio fetches: each one is a rate-limited TMDB round-trip (250 ms
//     apart, casts run to 50 people) - holding a row-locking transaction
//     open across them would stall every concurrent writer of those rows.
func (e *Enricher) EnrichItem(ctx context.Context, item *domainlib.MediaItem, result *domainmetadata.MetadataResult) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.EnrichItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(item.ID)))
	applyBasicFields(item, result)

	var artworkRecords []*domainmetadata.ArtworkRecord
	var bioPending []*domainlib.Person
	apply := func(ctx context.Context) error {
		if err := e.itemUpdater.Update(ctx, item); err != nil {
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

	// Post-commit side effects - the rows they reference are now durable.
	if e.artworkPostSaveHook != nil {
		for _, rec := range artworkRecords {
			e.artworkPostSaveHook(ctx, rec)
		}
	}
	for _, person := range bioPending {
		e.fetchPersonBio(ctx, person)
	}

	e.linkCollection(ctx, string(item.ID), result.Collection)
	// Fetch translations for all supported locales. Only for movies and series:
	// a season/episode's ProviderIDs["tmdb"] is its own object id, which lives in
	// a different id space than series ids - calling /tv/{id} with it 404s or,
	// worse, collides with an unrelated series and stores its title/overview onto
	// the season/episode. Per-season/episode translation needs the parent series
	// id + season/episode numbers (TODO); skip rather than corrupt until then.
	if item.Catalog != nil && item.Type != "season" && item.Type != "episode" {
		if tmdbID := item.Catalog.ProviderIDs["tmdb"]; tmdbID != "" {
			mediaType := "movie"
			if item.Type == "series" {
				mediaType = "tv"
			}
			e.enrichTranslations(ctx, item, tmdbID, mediaType)
		}
	}
	e.enrichPluginTexts(ctx, item)
	// Index LAST so the search doc carries the translated titles that
	// enrichTranslations just populated on the item (a user can then find a
	// title only stored in their own language).
	e.indexSearch(ctx, item, result.People)
	return nil
}

func applyBasicFields(item *domainlib.MediaItem, result *domainmetadata.MetadataResult) {
	// A season has no name of its own: its label is derived from its index
	// number (scan writes "Season N", clients render "Saison N" from
	// index_number). Providers hand one back anyway - TMDB returns "Gigantor"
	// for an Alias season, a japanese title for a Banshee one - and writing it
	// renamed seasons across the whole catalogue. Season is therefore the one
	// type whose stored title enrichment leaves alone; everything below
	// (overview, air date, provider ids, artwork upstream) still applies.
	if item.Type != domainlib.Season {
		item.Title = result.Title
	}
	item.Overview = result.Overview
	// OriginalTitle and Tagline are preserved when the provider returns nothing:
	// TMDB season/episode responses carry neither, so blanking them on a re-enrich
	// would wipe values an earlier movie/series-level pass had set.
	if result.OriginalTitle != "" {
		item.OriginalTitle = result.OriginalTitle
	}
	if result.Tagline != "" {
		item.Tagline = result.Tagline
	}

	if item.Catalog == nil {
		item.Catalog = &domainlib.MediaCatalog{}
	}
	// Same rationale as OriginalTitle/Tagline: don't clear existing genres when
	// the provider (e.g. a TMDB episode lookup) returns none.
	if len(result.Genres) > 0 {
		item.Catalog.Genres = result.Genres
	}
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
// persisted records. It does NOT fire artworkPostSaveHook - that is the
// caller's post-commit duty (firing inside the transaction would trigger an
// auto-download for rows a later failure rolls back).
func (e *Enricher) replaceArtwork(ctx context.Context, mediaItemID string, artwork []domainmetadata.ArtworkResult) ([]*domainmetadata.ArtworkRecord, error) {
	// Idempotency: enrichment re-runs constantly (daily re-enrich, every
	// "refresh metadata", scan-triggered re-adds). If the provider returned the
	// SAME artwork set we already store, do nothing - don't delete+recreate the
	// rows (DB churn) and don't return records (which would re-fire the download
	// hook). Re-downloading artwork we already have was the root cause of the
	// runaway artwork storage; the right behaviour is to only act on a real
	// change.
	if len(artwork) == 0 {
		// Provider returned no artwork - almost always a transient miss or a
		// match without images. Don't wipe artwork we already have (posters
		// would vanish until the next enrich re-adds them = churn + bad UX).
		// Explicit clearing goes through resetSingleItem / ResearchMetadata,
		// which delete directly, not here.
		return nil, nil
	}
	if existing, err := e.artworkQuery.GetByMediaItem(ctx, mediaItemID); err == nil && artworkSetUnchanged(existing, artwork) {
		return nil, nil
	}
	// Build the new records BEFORE deleting the old ones. If the provider gave
	// artwork but none of it produced a valid record (bad type/URL), bail out
	// without deleting - otherwise we'd wipe the existing artwork and insert
	// nothing, leaving the item with no poster until the next successful enrich.
	records := make([]*domainmetadata.ArtworkRecord, 0, len(artwork))
	for i, art := range artwork {
		if rec := buildArtworkRecord(mediaItemID, i, art); rec != nil {
			records = append(records, rec)
		}
	}
	if len(records) == 0 {
		return nil, nil
	}
	if err := e.artworkCmd.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		return nil, fmt.Errorf("delete old artwork: %w", err)
	}
	if err := e.artworkCmd.CreateBatch(ctx, records); err != nil {
		return nil, fmt.Errorf("create artwork batch: %w", err)
	}
	return records, nil
}

// artworkSetUnchanged reports whether the persisted records cover exactly the
// same (type, source URL) multiset as the incoming provider results - i.e. the
// provider returned nothing new and there is nothing to do.
func artworkSetUnchanged(existing []*domainmetadata.ArtworkRecord, incoming []domainmetadata.ArtworkResult) bool {
	if len(existing) != len(incoming) {
		return false
	}
	have := make(map[string]int, len(existing))
	for _, r := range existing {
		url := ""
		if r.SourceURL != nil {
			url = *r.SourceURL
		}
		have[r.Type+"\x00"+url]++
	}
	for _, a := range incoming {
		k := a.Type + "\x00" + a.URL
		if have[k] == 0 {
			return false
		}
		have[k]--
	}
	return true
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
		slog.WarnContext(ctx, "preload persons failed, falling back to per-row lookup", slog.Any("err", err))
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
	// Build the batch BEFORE deleting the old links. If the provider returned no
	// people (or none survived dedup), skip the delete entirely - otherwise we'd
	// wipe the existing cast/crew and insert nothing, leaving the item with no
	// cast until the next successful enrich. Same data-loss class as the
	// replaceArtwork no-wipe guard (errors mid-loop already returned above, so a
	// non-uow caller no longer commits the delete before failing).
	if len(batch) == 0 {
		return bioPending, nil
	}
	if err := e.mediaPersons.DeleteByMediaItemID(ctx, shared.MediaItemID(mediaItemID)); err != nil {
		return nil, fmt.Errorf("delete old media_people: %w", err)
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

// personNeedsBio reports whether the provider should be asked for this
// person's biography: it is missing AND either never checked, or last
// checked longer than bioRecheckTTL ago. A person the provider genuinely has
// no bio for is otherwise re-fetched on every re-enrichment pass, forever.
func (e *Enricher) personNeedsBio(p *domainlib.Person) bool {
	if p.Biography != "" {
		return false
	}
	if p.BioCheckedAt == nil {
		return true
	}
	clock := e.now
	if clock == nil {
		clock = time.Now
	}
	return clock().Sub(*p.BioCheckedAt) >= bioRecheckTTL
}

// findOrCreatePerson takes the preloaded person (may be nil when absent
// from DB) and either enriches the existing row with new provider IDs/
// thumbnails, or creates a fresh row. needsBio reports that the row has a
// TMDB ID but no biography yet - the CALLER fetches it after the enrichment
// transaction commits (a TMDB round-trip must not run under row locks).
func (e *Enricher) findOrCreatePerson(ctx context.Context, p domainmetadata.PersonResult, preloaded *domainlib.Person) (person *domainlib.Person, needsBio bool, err error) {
	if preloaded != nil {
		person = e.refreshKnownPerson(ctx, p, preloaded)
		// needsBio no longer requires a tmdb id: the bio lookup is by NAME
		// (see fetchPersonBio), so the id gated nothing but the attempt.
		return person, e.personNeedsBio(person), nil
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
				slog.WarnContext(ctx, "update person failed", slog.String("personID", person.ID), slog.Any("err", err))
			}
		}
		return person, e.personNeedsBio(person), nil
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
	// A freshly created person always needs its bio: the lookup is by name.
	return person, true, nil
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
			slog.WarnContext(ctx, "update person failed", slog.String("personID", person.ID), slog.Any("err", err))
		}
	}
	return person
}

// fetchPersonBio fetches biography from the primary provider and updates the person record.
func (e *Enricher) fetchPersonBio(ctx context.Context, person *domainlib.Person) {
	// The one gate that means something is the capability check below: the bio
	// comes from GetPersonDetails BY NAME on the primary provider. The two
	// preconditions that used to sit here - TranslationProvider() present and
	// ProviderIDs["tmdb"] non-empty - policed capabilities this function does
	// not use, and each silently produced people whose empty bio read as "the
	// provider has none". Lifted 2026-08-25 (decision taken, not deferred).
	//
	// The biography comes from the person DETAIL endpoint. This used to open
	// with FetchPersonTranslations(id, []string{""}) whose own comment said it
	// "returns nothing useful" and whose result was discarded on the next line
	// - so its only remaining effect was to skip the fetch below whenever it
	// failed. One rate-limited call nobody read was enough to leave a person
	// with an empty bio that reads as "the provider has none".
	if fetcher, ok2 := capabilityFrom[personDetailFetcher](e.chain.Primary()); ok2 {
		details, detErr := fetcher.GetPersonDetails(ctx, person.Name)
		if detErr != nil {
			// A swallowed 429/timeout here is indistinguishable from "the
			// provider has no bio" - log it so the empty bio isn't read as fact.
			// Do NOT stamp BioCheckedAt: a transient failure must not silence
			// this person from being retried for the next bioRecheckTTL.
			slog.WarnContext(ctx, "person bio fetch failed", slog.String("person", person.Name), slog.Any("err", detErr))
			return
		}
		if details != nil && details.Biography != "" {
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
		}
		// The provider answered (with or without a biography): stamp the
		// negative-cache marker so a genuinely bio-less person is not asked
		// again until bioRecheckTTL elapses.
		now := e.now
		if now == nil {
			now = time.Now
		}
		checkedAt := now()
		person.BioCheckedAt = &checkedAt
		if err := e.persons.Update(ctx, person); err != nil {
			slog.WarnContext(ctx, "update person bio failed", slog.String("personID", person.ID), slog.Any("err", err))
		}
	}
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
	item.CastNames = names
	if err := e.search.IndexMediaItem(ctx, item); err != nil {
		slog.WarnContext(ctx, "index media item", slog.Any("err", err))
	}
}

func (e *Enricher) linkCollection(ctx context.Context, mediaItemID string, ref *domainmetadata.CollectionRef) {
	if e.collections == nil || ref == nil {
		return
	}
	col, err := e.collections.Upsert(ctx, ref.TmdbID, ref.Name, "", ref.PosterURL, ref.BackdropURL)
	if err != nil {
		slog.WarnContext(ctx, "upsert collection", slog.Int("tmdb_id", ref.TmdbID), slog.Any("err", err))
		return
	}
	if col == nil {
		return
	}
	if err := e.collections.LinkMediaItem(ctx, col.ID, mediaItemID, 0); err != nil {
		slog.WarnContext(ctx, "link media item to collection", slog.String("collection_id", col.ID), slog.String("media_item_id", mediaItemID), slog.Any("err", err))
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

// ClearTranslations removes every stored locale translation for an item. Used
// when correcting a wrong match (ResearchMetadata): the old translation rows
// belong to the bad match and would otherwise overlay the corrected base title
// via the read path (e.g. a season left showing a foreign show's title even
// after its base was fixed). Re-enrichment repopulates them for a valid match.
func (e *Enricher) ClearTranslations(ctx context.Context, mediaItemID string) {
	if e.mediaTranslations == nil {
		return
	}
	if err := e.mediaTranslations.DeleteByMediaItem(ctx, mediaItemID); err != nil {
		slog.WarnContext(ctx, "clear media translations", slog.String("item", mediaItemID), slog.Any("err", err))
	}
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
		slog.WarnContext(ctx, "fetch translations", slog.String("item", string(item.ID)), slog.Any("err", err))
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
		slog.WarnContext(ctx, "upsert media translations batch", slog.String("item", string(item.ID)), slog.Any("err", err))
		return
	}
	// Carry the translations on the item so the post-enrich search index picks
	// up the translated titles without re-reading them from Postgres.
	translations := make([]domainlib.MediaItemTranslation, 0, len(batch))
	for _, b := range batch {
		translations = append(translations, *b)
	}
	item.Translations = translations
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
		slog.WarnContext(ctx, "fetch person translations", slog.String("person", person.ID), slog.Any("err", err))
		return
	}
	for locale, bio := range bios {
		if err := e.personTranslations.Upsert(ctx, &domainlib.PersonTranslation{
			PersonID:  person.ID,
			Locale:    locale,
			Biography: bio,
		}); err != nil {
			slog.WarnContext(ctx, "upsert person translation", slog.String("locale", locale), slog.Any("err", err))
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

// enrichPluginTexts asks the metadata plugins about a movie or a series and
// stores each answer beside the TMDB text, replacing what that plugin gave
// before. A plugin that failed keeps its old text; the read path decides which
// fields show (the administrator's choice per plugin). Seasons and episodes are
// left to TMDB: their numbering is the series', which a plugin's catalogue may
// not share.
func (e *Enricher) enrichPluginTexts(ctx context.Context, item *domainlib.MediaItem) {
	if e.pluginSource == nil || e.pluginTexts == nil {
		return
	}
	if item.Type != domainlib.MovieItem && item.Type != domainlib.Series {
		return
	}
	req := domainmetadata.PluginTextRequest{
		MediaItemID:   string(item.ID),
		Type:          string(item.Type),
		Title:         item.Title,
		OriginalTitle: item.OriginalTitle,
		Language:      e.textLanguage(),
		Locales:       e.translationLocales,
	}
	if item.Catalog != nil {
		req.ProviderIDs = item.Catalog.ProviderIDs
		if item.Catalog.Year != nil {
			req.Year = *item.Catalog.Year
		}
	}
	for _, answer := range e.pluginSource.LookupTexts(ctx, req) {
		texts := make([]domainlib.PluginText, 0, len(answer.Texts))
		for _, t := range answer.Texts {
			texts = append(texts, domainlib.PluginText{
				MediaItemID: string(item.ID), PluginID: answer.PluginID, Locale: t.Locale,
				Title: t.Title, Overview: t.Overview, Tagline: t.Tagline,
			})
		}
		if err := e.pluginTexts.ReplaceForItem(ctx, string(item.ID), answer.PluginID, texts); err != nil {
			slog.WarnContext(ctx, "store plugin metadata texts",
				slog.String("item", string(item.ID)), slog.String("plugin", answer.PluginID), slog.Any("err", err))
		}
	}
}

// textLanguage is the locale of the TMDB text (the primary provider's
// language, "en-US" read as "en"), "en" when it does not say.
func (e *Enricher) textLanguage() string {
	lang := ""
	if p := e.chain.Primary(); p != nil {
		var inner domainmetadata.MetadataProvider = p
		for {
			if la, ok := inner.(domainmetadata.LanguageAware); ok {
				lang = la.Language()
				break
			}
			u, ok := inner.(domainmetadata.ProviderUnwrapper)
			if !ok {
				break
			}
			inner = u.Unwrap()
		}
	}
	if i := strings.IndexAny(lang, "-_"); i >= 0 {
		lang = lang[:i]
	}
	if lang == "" {
		return "en"
	}
	return strings.ToLower(lang)
}
