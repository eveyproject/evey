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
		// Same negative cache as the per-item enrichment: a person the provider
		// had no biography for is not asked again before bioRecheckTTL.
		if !e.personNeedsBio(person) {
			continue
		}
		details, detErr := fetcher.GetPersonDetails(ctx, person.Name)
		if detErr != nil {
			// Transient (429, timeout): leave the stamp alone so the next pass
			// retries instead of silencing the person for a month.
			if log != nil {
				log.Debug("bio fetch failed", slog.String("name", person.Name), slog.Any("err", detErr))
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if details == nil || details.Biography == "" {
			// A real "no bio" answer is remembered, otherwise this loop and the
			// daily re-enrichment repeat the round-trip for every bio-less
			// person, forever.
			checked := e.clockNow()
			person.BioCheckedAt = &checked
			if updateErr := e.persons.Update(ctx, person); updateErr != nil {
				slog.WarnContext(ctx, "stamp person bio check", slog.String("name", person.Name), slog.Any("err", updateErr))
			}
			if log != nil {
				log.Debug("no bio found", slog.String("name", person.Name))
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		checked := e.clockNow()
		person.BioCheckedAt = &checked
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
