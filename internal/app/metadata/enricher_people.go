package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

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
