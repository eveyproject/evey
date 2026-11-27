package library

import (
	"context"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// ExpectedEpisode is one episode a series has according to its catalog
// source (TMDB), owned or not. Season 0 holds the specials.
type ExpectedEpisode struct {
	Season  int
	Episode int
	Title   string
	AirDate *time.Time // nil when not announced
}

// CollectionPart is one film of a saga (a TMDB collection), owned or not.
type CollectionPart struct {
	TmdbID      int
	Title       string
	ReleaseDate *time.Time
	PosterURL   string
}

// CatalogSource reads what a series or a saga contains. It is the server's
// own source (TMDB): plugins never reach it, they read the gaps Evey works
// out from what it stored.
type CatalogSource interface {
	SeriesEpisodes(ctx context.Context, tmdbSeriesID string) ([]ExpectedEpisode, error)
	CollectionParts(ctx context.Context, tmdbCollectionID int) ([]CollectionPart, error)
}

// SeriesRef is a series to check: its item and its TMDB id.
type SeriesRef struct {
	ItemID  string
	TmdbID  string
	ImdbID  string
	Title   string
	Year    int
	Library shared.LibraryID
}

// CollectionRef is a saga to check: its collection and TMDB id.
type CollectionRef struct {
	ID     string
	TmdbID int
	Name   string
}

// OwnedEpisode is an episode the library holds, by number.
type OwnedEpisode struct {
	SeriesID string
	Season   int
	Episode  int
}

// CatalogRepo stores what the catalog source said and when, and reads what
// the libraries own, so the gaps are worked out without calling the source.
type CatalogRepo interface {
	// SeriesToCheck lists series with a TMDB id never checked or checked
	// before olderThan, oldest first, at most limit.
	SeriesToCheck(ctx context.Context, olderThan time.Time, limit int) ([]SeriesRef, error)
	ReplaceSeriesEpisodes(ctx context.Context, seriesID string, episodes []ExpectedEpisode, checkedAt time.Time) error
	// CollectionsToCheck lists sagas with a TMDB id never checked or checked
	// before olderThan, oldest first, at most limit.
	CollectionsToCheck(ctx context.Context, olderThan time.Time, limit int) ([]CollectionRef, error)
	ReplaceCollectionParts(ctx context.Context, collectionID string, parts []CollectionPart, checkedAt time.Time) error

	// ListSeries lists every series (with or without a TMDB id), restricted
	// to libraryIDs when not empty.
	ListSeries(ctx context.Context, libraryIDs []shared.LibraryID) ([]SeriesRef, error)
	// ExpectedEpisodes returns the stored episodes per series item id.
	ExpectedEpisodes(ctx context.Context, seriesIDs []string) (map[string][]ExpectedEpisode, error)
	// OwnedEpisodes returns the numbered episodes per series item id.
	OwnedEpisodes(ctx context.Context, seriesIDs []string) (map[string][]OwnedEpisode, error)
	// SagasOwned lists the sagas that hold at least one owned film, with the
	// TMDB ids of the films owned, restricted to libraryIDs when not empty.
	SagasOwned(ctx context.Context, libraryIDs []shared.LibraryID) ([]CollectionRef, map[string][]int, error)
	// CollectionParts returns the stored parts per collection id.
	CollectionParts(ctx context.Context, collectionIDs []string) (map[string][]CollectionPart, error)
}
