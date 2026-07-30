package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// EnrichAllPeopleBios fetches bios for all people missing biography data
// using the person detail fetcher (TMDB search by name → details).
func (e *Enricher) EnrichAllPeopleBios(ctx context.Context, log *slog.Logger) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.EnrichAllPeopleBios")
	defer span.End()
	fetcher := e.personFetcher
	if fetcher == nil {
		err := errors.New("person fetcher not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return 0, err
	}
	all, err := e.persons.List(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return 0, fmt.Errorf("list people: %w", err)
	}
	span.SetAttributes(attribute.Int("persons.count", len(all)))
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
			slog.WarnContext(ctx, "update person bio", slog.String("name", person.Name), slog.Any("err", updateErr))
		} else {
			enriched++
			if log != nil {
				log.Debug("enriched person bio", slog.String("name", person.Name))
			}
		}
		// Rate limit: ~2 req/s to avoid TMDB throttling
		time.Sleep(500 * time.Millisecond)
	}
	span.SetAttributes(attribute.Int("persons.enriched", enriched))
	return enriched, nil
}
