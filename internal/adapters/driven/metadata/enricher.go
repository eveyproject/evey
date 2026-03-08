package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	domaindomain "github.com/gravity-zero/evey/internal/domain/library"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
)

// Enricher stores metadata results (artwork, people) into the DB.
type Enricher struct {
	artwork      domainmetadata.ArtworkRepository
	persons      domaindomain.PersonRepository
	mediaPersons domaindomain.MediaPersonRepository
	mediaItems   domaindomain.MediaItemRepository
}

func NewEnricher(
	artwork domainmetadata.ArtworkRepository,
	persons domaindomain.PersonRepository,
	mediaPersons domaindomain.MediaPersonRepository,
	mediaItems domaindomain.MediaItemRepository,
) *Enricher {
	return &Enricher{
		artwork:      artwork,
		persons:      persons,
		mediaPersons: mediaPersons,
		mediaItems:   mediaItems,
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
	item.UpdatedAt = time.Now()

	if item.ProviderIDs == nil {
		item.ProviderIDs = make(map[string]string)
	}
	for k, v := range result.ProviderIDs {
		item.ProviderIDs[k] = v
	}

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
		rec := &domainmetadata.ArtworkRecord{
			ID:          uuid.New().String(),
			MediaItemID: item.ID,
			Type:        art.Type,
			Provider:    "tmdb",
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
	for i, p := range result.People {
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
			SortOrder:     i,
		}
		if createErr := e.mediaPersons.Create(ctx, mp); createErr != nil {
			_ = createErr
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

// AutoEnrichItem searches TMDB and enriches a single item.
// Skips items that already have an overview, EXCEPT episodes (always re-fetch real episode title).
func (e *Enricher) AutoEnrichItem(ctx context.Context, item *domaindomain.MediaItem, tmdb *TMDBProvider, log *slog.Logger) {
	if item.Title == "" {
		return
	}

	// Episodes: fetch specific episode data from TMDB using parent chain
	if item.Type == string(domaindomain.Episode) {
		e.autoEnrichEpisode(ctx, item, tmdb, log)
		return
	}

	// Non-episode: skip if already enriched
	if item.Overview != "" {
		return
	}

	mediaType := "movie"
	if item.Type == string(domaindomain.Series) {
		mediaType = "tv"
	}
	results, err := tmdb.Search(ctx, item.Title, mediaType)
	if err != nil || len(results) == 0 {
		return
	}
	best := results[0]
	if item.Year != nil {
		for _, r := range results {
			if r.Year != nil && *r.Year == *item.Year {
				best = r
				break
			}
		}
	}
	tmdbID, ok := best.ProviderIDs["tmdb"]
	if !ok {
		return
	}
	full, err := tmdb.GetByProviderID(ctx, tmdbID, mediaType)
	if err != nil {
		full = &best
	}
	if err := e.EnrichItem(ctx, item, full); err != nil && log != nil {
		log.Warn("auto-enrich failed", slog.String("title", item.Title), slog.Any("err", err))
	}
}

// autoEnrichEpisode fetches the specific episode title/overview from TMDB
// by walking up the parent chain (episode → season → series → TMDB ID).
func (e *Enricher) autoEnrichEpisode(ctx context.Context, item *domaindomain.MediaItem, tmdb *TMDBProvider, log *slog.Logger) {
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
	seriesTMDB, ok := series.ProviderIDs["tmdb"]
	if !ok || seriesTMDB == "" {
		// Try to enrich series first so we get its TMDB ID
		if series.Overview == "" {
			e.AutoEnrichItem(ctx, series, tmdb, log)
			// Re-fetch after enrichment
			series, err = e.mediaItems.GetByID(ctx, season.ParentID)
			if err != nil {
				return
			}
			seriesTMDB, ok = series.ProviderIDs["tmdb"]
			if !ok || seriesTMDB == "" {
				return
			}
		} else {
			return
		}
	}
	result, err := tmdb.GetEpisodeDetails(ctx, seriesTMDB, *season.IndexNumber, *item.IndexNumber)
	if err != nil {
		if log != nil {
			log.Debug("episode tmdb lookup failed", slog.String("series_tmdb", seriesTMDB),
				slog.Int("season", *season.IndexNumber), slog.Int("ep", *item.IndexNumber), slog.Any("err", err))
		}
		return
	}
	// Preserve existing genres/studios from series for display context
	if err := e.EnrichItem(ctx, item, result); err != nil && log != nil {
		log.Warn("episode enrich failed", slog.String("id", item.ID), slog.Any("err", err))
	}
}

// AutoEnrichAll enriches all items with no overview concurrently (max 3 at a time).
func (e *Enricher) AutoEnrichAll(ctx context.Context, items []*domaindomain.MediaItem, tmdb *TMDBProvider, log *slog.Logger) {
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
			e.AutoEnrichItem(ctx, it, tmdb, log)
		}(item)
	}
	wg.Wait()
}
