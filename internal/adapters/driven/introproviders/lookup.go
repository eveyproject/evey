// Package introproviders wires external intro/recap/credits metadata
// sources (TIDB, IntroDB) behind the domainlibrary.IntroMetadataProvider
// port. Two upstream services with complementary coverage are queried in
// chain order; results are cached in Postgres so a typical scan only hits
// the network once per episode for the whole catalog lifetime.
package introproviders

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Lookup is the resolved set of identifiers needed to query the upstream
// providers. Filled by AncestorResolver from a MediaItem.
//
//   - Movies: TmdbID = movie ID, Season/Episode = 0, IsMovie = true.
//   - TV episodes: TmdbID = series ID (NOT episode ID), Season + Episode
//     = 1-indexed numbers, IsMovie = false.
type Lookup struct {
	TmdbID  int    // TIDB primary key for both movies and TV series
	ImdbID  string // optional, may be ""; IntroDB primary key + TIDB v3 fallback
	Season  int    // 0 for movies
	Episode int    // 0 for movies
	IsMovie bool   // true → movie endpoint shape; false → TV episode shape
	// DurationMs is the item's runtime in milliseconds (0 = unknown). Sent
	// to TIDB v3 as duration_ms to pick the matching release cut, and used
	// to close open-ended credits/preview segments (end_ms == null = "to end
	// of media") against a concrete end time.
	DurationMs int64
	MediaItem  *domainlibrary.MediaItem
}

// MediaAncestorRepo is the read-only slice of MediaItemRepository the
// resolver needs. Defined as its own interface so the resolver can be
// fed a fake or a strictly-typed query repo.
type MediaAncestorRepo interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error)
}

// AncestorResolver walks the parent chain of an Episode (Episode → Season
// → Series) to recover the Series-level TMDB / IMDb IDs that TIDB and
// IntroDB key on. For Movies it returns the item's own IDs.
type AncestorResolver struct {
	repo MediaAncestorRepo
}

// NewAncestorResolver returns a resolver backed by the given repo. The
// repo MUST resolve MediaItem IDs to fully-loaded items (Catalog +
// IndexNumber populated) since the resolver dereferences both fields.
func NewAncestorResolver(repo MediaAncestorRepo) *AncestorResolver {
	return &AncestorResolver{repo: repo}
}

// ErrLookupUnresolvable means the item has no usable TMDB/IMDb ID - a
// hard skip for the providers (no fallback to title-search). Returned
// when the catalog wasn't enriched, parents weren't loaded, or the item
// is a container type (series / season).
var ErrLookupUnresolvable = errors.New("introproviders: media has no usable TMDB or IMDb ID")

// Resolve walks ancestors from item up to the series/movie root and
// returns the canonical Lookup. Errors short-circuit the providers; the
// caller falls back to chapter pass / heuristics.
func (r *AncestorResolver) Resolve(ctx context.Context, item *domainlibrary.MediaItem) (Lookup, error) {
	ctx, span := tracing.StartSpan(ctx, "introproviders.lookup.Resolve")
	defer span.End()

	if item == nil {
		return Lookup{}, ErrLookupUnresolvable
	}
	switch item.Type {
	case domainlibrary.MovieItem:
		ids := providerIDs(item)
		tmdbID, _ := strconv.Atoi(ids["tmdb"])
		if tmdbID == 0 && ids["imdb"] == "" {
			return Lookup{}, ErrLookupUnresolvable
		}
		return Lookup{
			TmdbID:     tmdbID,
			ImdbID:     ids["imdb"],
			IsMovie:    true,
			DurationMs: itemDurationMs(item),
			MediaItem:  item,
		}, nil
	case domainlibrary.Episode:
		if item.IndexNumber == nil || *item.IndexNumber == 0 {
			return Lookup{}, ErrLookupUnresolvable
		}
		ep := *item.IndexNumber
		// Episode → Season (parent), Season → Series (grandparent).
		season, err := r.repo.GetByID(ctx, shared.MediaItemID(item.ParentID))
		if err != nil || season == nil || season.IndexNumber == nil {
			return Lookup{}, fmt.Errorf("resolve season: %w", err)
		}
		series, err := r.repo.GetByID(ctx, shared.MediaItemID(season.ParentID))
		if err != nil || series == nil {
			return Lookup{}, fmt.Errorf("resolve series: %w", err)
		}
		ids := providerIDs(series)
		tmdbID, _ := strconv.Atoi(ids["tmdb"])
		if tmdbID == 0 && ids["imdb"] == "" {
			return Lookup{}, ErrLookupUnresolvable
		}
		return Lookup{
			TmdbID:     tmdbID,
			ImdbID:     ids["imdb"],
			Season:     *season.IndexNumber,
			Episode:    ep,
			IsMovie:    false,
			DurationMs: itemDurationMs(item),
			MediaItem:  item,
		}, nil
	default:
		// Series / Season / Album / Track / Photo: no per-item segment data.
		return Lookup{}, ErrLookupUnresolvable
	}
}

// itemDurationMs returns the item's on-disk runtime in milliseconds, or 0
// when no file is loaded (container types, or a repo that didn't JOIN
// media_files). 0 is a safe sentinel: the providers omit duration_ms and
// fall back to dropping open-ended segments.
func itemDurationMs(item *domainlibrary.MediaItem) int64 {
	if item == nil || item.File == nil {
		return 0
	}
	return item.File.Duration.Milliseconds()
}

func providerIDs(item *domainlibrary.MediaItem) map[string]string {
	if item == nil || item.Catalog == nil || item.Catalog.ProviderIDs == nil {
		return map[string]string{}
	}
	return item.Catalog.ProviderIDs
}

// Client is the narrow contract every upstream HTTP adapter (TIDB,
// IntroDB) implements. Returning an empty slice with nil error means
// "service responded but has no data for this lookup" - the chain
// continues to the next provider.
type Client interface {
	Source() string // stable identifier for MediaSegment.Source ("tidb", "introdb")
	Fetch(ctx context.Context, lookup Lookup) ([]domainlibrary.MediaSegment, error)
}
