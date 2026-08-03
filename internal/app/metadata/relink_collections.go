package metadata

import (
	"context"
	"fmt"
	"log/slog"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// RelinkReport is the outcome of one saga-repair sweep.
type RelinkReport struct {
	// Scanned is how many unlinked movies the sweep looked at.
	Scanned int
	// Linked is how many joined a saga. A standalone film is scanned without
	// being linked - that is the ordinary case, not a failure.
	Linked int
	// Failed is how many could not be resolved or linked. A failure never
	// aborts the sweep: one unreachable title must not cost the repair of
	// every title behind it.
	Failed int
}

// RelinkCollections repairs saga membership for movies that hold a provider id
// and belong to no collection.
//
// Membership is only written while a title is being enriched
// (EnrichItem -> linkCollection), so every movie enriched before that path
// existed never joined its saga - and nothing ever went back for them. On the
// production server that left 230 of 236 auto-created sagas listing a single
// title while the sequels sat in the same library (measured 2026-08-04).
//
// Idempotent by construction: the work list is "movies in no collection", so a
// second run has nothing to redo, and both the upsert and the link collapse on
// conflict. A standalone film is re-examined on every run - the cost of not
// persisting a negative that the provider may later fill in.
func (e *Enricher) RelinkCollections(ctx context.Context, limit int) (RelinkReport, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RelinkCollections")
	defer span.End()

	var rep RelinkReport
	if e.collections == nil || e.chain == nil {
		return rep, nil
	}
	todo, err := e.collections.ListMoviesWithoutCollection(ctx, limit)
	if err != nil {
		span.RecordError(err)
		return rep, fmt.Errorf("RelinkCollections: list candidates: %w", err)
	}
	if len(todo) == 0 {
		return rep, nil
	}
	provider := e.chain.Primary()
	if provider == nil {
		return rep, nil
	}
	bgtasks.Register("collection_relink")
	slog.InfoContext(ctx, "collection relink: sweep started", slog.Int("candidates", len(todo)))

	for _, m := range todo {
		select {
		case <-ctx.Done():
			return rep, ctx.Err()
		default:
		}
		rep.Scanned++
		linked, err := e.relinkOne(ctx, provider, m)
		switch {
		case err != nil:
			rep.Failed++
			slog.WarnContext(ctx, "collection relink: title skipped",
				slog.String("media_item_id", string(m.MediaItemID)),
				slog.String("provider_id", m.ProviderID),
				slog.Any("err", err))
		case linked:
			rep.Linked++
		}
	}
	slog.InfoContext(ctx, "collection relink: sweep finished",
		slog.Int("scanned", rep.Scanned), slog.Int("linked", rep.Linked), slog.Int("failed", rep.Failed))
	return rep, nil
}

// relinkOne reports whether the title joined a saga. A standalone film yields
// (false, nil): scanned, untouched, not a failure.
func (e *Enricher) relinkOne(ctx context.Context, provider domainmetadata.MetadataProvider, m domainlib.UnlinkedMovie) (bool, error) {
	defer bgtasks.Begin("collection_relink", string(m.MediaItemID))()
	if e.apiLimiter != nil {
		if err := e.apiLimiter.Wait(ctx); err != nil {
			return false, err
		}
	}
	res, err := provider.GetByProviderID(ctx, m.ProviderID, "movie")
	if err != nil {
		return false, err
	}
	if res == nil || res.Collection == nil {
		return false, nil
	}
	ref := res.Collection
	col, err := e.collections.Upsert(ctx, ref.TmdbID, ref.Name, "", ref.PosterURL, ref.BackdropURL)
	if err != nil {
		return false, err
	}
	if col == nil {
		return false, nil
	}
	if err := e.collections.LinkMediaItem(ctx, col.ID, string(m.MediaItemID), 0); err != nil {
		return false, err
	}
	return true, nil
}
