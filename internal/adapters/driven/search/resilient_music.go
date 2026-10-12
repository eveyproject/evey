package search

import (
	"context"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// ResilientMusicSearch wraps the Meilisearch music index (primary) with a SQL
// fallback (the track repo's ILIKE search). Writes go to the primary
// best-effort; reads try the primary and fall back to SQL when it errors or is
// absent. Mirrors ResilientSearch for media.
type ResilientMusicSearch struct {
	primary  music.MusicSearchIndex
	fallback music.MusicSearchIndex
}

func NewResilientMusic(primary, fallback music.MusicSearchIndex) *ResilientMusicSearch {
	return &ResilientMusicSearch{primary: primary, fallback: fallback}
}

func (r *ResilientMusicSearch) Primary() music.MusicSearchIndex { return r.primary }

func (r *ResilientMusicSearch) IndexTrack(ctx context.Context, t music.Track) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.music.IndexTrack")
	defer span.End()
	if r.primary == nil {
		return nil
	}
	if err := r.primary.IndexTrack(ctx, t); err != nil {
		slog.Warn("music search index: primary failed, skipping", slog.Any("err", err))
	}
	return nil
}

func (r *ResilientMusicSearch) IndexTracksBatch(ctx context.Context, tracks []music.Track) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.music.IndexTracksBatch")
	defer span.End()
	if r.primary == nil {
		return nil
	}
	if err := r.primary.IndexTracksBatch(ctx, tracks); err != nil {
		slog.Warn("music search index batch: primary failed, skipping", slog.Any("err", err))
	}
	return nil
}

func (r *ResilientMusicSearch) RemoveTrack(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.music.RemoveTrack")
	defer span.End()
	if r.primary == nil {
		return nil
	}
	if err := r.primary.RemoveTrack(ctx, id); err != nil {
		slog.Warn("music search remove: primary failed, skipping", slog.Any("err", err))
	}
	return nil
}

func (r *ResilientMusicSearch) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.music.Search")
	defer span.End()
	if r.primary != nil {
		tracks, err := r.primary.Search(ctx, libraryID, query, limit)
		if err == nil {
			return tracks, nil
		}
		slog.Warn("music search: primary failed, falling back to SQL", slog.Any("err", err))
	}
	if r.fallback == nil {
		return nil, nil
	}
	return r.fallback.Search(ctx, libraryID, query, limit)
}

// musicRepoFallback adapts the track repository's SQL ILIKE search to the
// MusicSearchIndex port. Index/Remove are no-ops: the SQL table IS the source of
// truth, there is no separate index to maintain.
type musicRepoFallback struct {
	repo music.TrackRepository
}

// NewMusicRepoFallback builds the SQL fallback music search from the track repo.
func NewMusicRepoFallback(repo music.TrackRepository) music.MusicSearchIndex {
	return &musicRepoFallback{repo: repo}
}

func (f *musicRepoFallback) IndexTrack(context.Context, music.Track) error         { return nil }
func (f *musicRepoFallback) IndexTracksBatch(context.Context, []music.Track) error { return nil }
func (f *musicRepoFallback) RemoveTrack(context.Context, string) error             { return nil }

func (f *musicRepoFallback) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]music.Track, error) {
	// repo.Search treats an empty libraryID as a cross-library search, so the
	// global search bar still returns results when Meilisearch is down. Access
	// is enforced by the caller (handler post-filters by allowed libraries).
	return f.repo.Search(ctx, libraryID, query, limit)
}
