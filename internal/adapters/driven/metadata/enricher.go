package metadata

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
)

// Enricher fetches TMDB metadata for media items and persists it.
type Enricher struct {
	tmdb       *TMDBProvider
	items      domainlibrary.MediaItemRepository
	artwork    domainmeta.ArtworkRepository
	log        *slog.Logger
	sem        chan struct{} // concurrency limiter
}

func NewEnricher(
	tmdb *TMDBProvider,
	items domainlibrary.MediaItemRepository,
	artwork domainmeta.ArtworkRepository,
	log *slog.Logger,
) *Enricher {
	return &Enricher{
		tmdb:    tmdb,
		items:   items,
		artwork: artwork,
		log:     log,
		sem:     make(chan struct{}, 3), // max 3 concurrent TMDB requests
	}
}

// EnrichItem fetches TMDB metadata for a single item and updates the DB.
// Skips items that already have an overview.
func (e *Enricher) EnrichItem(ctx context.Context, item *domainlibrary.MediaItem) {
	if item.Overview != "" {
		return
	}
	if item.Title == "" {
		return
	}

	e.sem <- struct{}{}
	defer func() { <-e.sem }()

	mediaType := "movie"
	if item.Type == string(domainlibrary.Episode) || item.Type == string(domainlibrary.Series) {
		mediaType = "tv"
	}

	results, err := e.tmdb.Search(ctx, item.Title, mediaType)
	if err != nil {
		e.log.Warn("tmdb search failed", slog.String("title", item.Title), slog.Any("err", err))
		return
	}
	if len(results) == 0 {
		return
	}

	// pick best match: prefer year match if we have one
	result := results[0]
	if item.Year != nil {
		for _, r := range results {
			if r.Year != nil && *r.Year == *item.Year {
				result = r
				break
			}
		}
	}

	now := time.Now()
	item.Title = result.Title
	item.OriginalTitle = result.OriginalTitle
	item.Overview = result.Overview
	item.Tagline = result.Tagline
	item.Genres = result.Genres
	item.Studios = result.Studios
	item.ProviderIDs = result.ProviderIDs
	item.PremiereDate = result.PremiereDate
	item.Year = result.Year
	item.CommunityRating = result.CommunityRating
	item.OfficialRating = result.OfficialRating
	item.UpdatedAt = now

	if err := e.items.Update(ctx, item); err != nil {
		e.log.Warn("update media item metadata failed", slog.String("id", item.ID), slog.Any("err", err))
		return
	}

	e.storeArtwork(ctx, item.ID, result.Artwork)
	e.log.Info("enriched", slog.String("title", item.Title), slog.String("id", item.ID))
}

func (e *Enricher) storeArtwork(ctx context.Context, mediaItemID string, artwork []domainmeta.ArtworkResult) {
	for i, a := range artwork {
		url := a.URL
		if url == "" {
			continue
		}
		rec := &domainmeta.ArtworkRecord{
			ID:          uuid.New().String(),
			MediaItemID: mediaItemID,
			Type:        a.Type,
			Provider:    "tmdb",
			SourceURL:   &url,
			SortOrder:   i,
		}
		if a.Width > 0 {
			rec.Width = &a.Width
		}
		if a.Height > 0 {
			rec.Height = &a.Height
		}
		if a.Language != "" {
			rec.Language = &a.Language
		}
		_ = e.artwork.Create(ctx, rec)
	}
}

// EnrichAll enriches all items with no overview concurrently.
func (e *Enricher) EnrichAll(ctx context.Context, items []*domainlibrary.MediaItem) {
	var wg sync.WaitGroup
	for _, item := range items {
		if item.Overview != "" {
			continue
		}
		wg.Add(1)
		go func(it *domainlibrary.MediaItem) {
			defer wg.Done()
			e.EnrichItem(ctx, it)
		}(item)
	}
	wg.Wait()
}
