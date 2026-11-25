package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// CatalogRepo stores what the catalog source says a series or a saga holds,
// and reads what the libraries own, for the missing episodes and films.
type CatalogRepo struct {
	db *sql.DB
}

func NewCatalogRepo(db *sql.DB) *CatalogRepo { return &CatalogRepo{db: db} }

var _ library.CatalogRepo = (*CatalogRepo)(nil)

func libraryIDStrings(ids []shared.LibraryID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func (r *CatalogRepo) SeriesToCheck(ctx context.Context, olderThan time.Time, limit int) ([]library.SeriesRef, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.SeriesToCheck")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mi.id, mc.provider_ids->>'tmdb', mi.title, COALESCE(mc.year, 0), mi.library_id
		   FROM media_items mi
		   JOIN media_catalog mc ON mc.media_item_id = mi.id
		   LEFT JOIN series_catalog_checks k ON k.series_item_id = mi.id
		  WHERE mi.type = 'series'
		    AND COALESCE(mc.provider_ids->>'tmdb', '') ~ '^[0-9]+$'
		    AND (k.checked_at IS NULL OR k.checked_at < $1)
		  ORDER BY k.checked_at NULLS FIRST, mi.id
		  LIMIT $2`, olderThan, limit)
	if err != nil {
		return nil, fmt.Errorf("CatalogRepo.SeriesToCheck: %w", err)
	}
	defer rows.Close()
	var out []library.SeriesRef
	for rows.Next() {
		var s library.SeriesRef
		var lib string
		if err := rows.Scan(&s.ItemID, &s.TmdbID, &s.Title, &s.Year, &lib); err != nil {
			return nil, fmt.Errorf("CatalogRepo.SeriesToCheck scan: %w", err)
		}
		s.Library = shared.LibraryID(lib)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *CatalogRepo) ReplaceSeriesEpisodes(ctx context.Context, seriesID string, episodes []library.ExpectedEpisode, checkedAt time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.ReplaceSeriesEpisodes")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("CatalogRepo.ReplaceSeriesEpisodes begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM series_expected_episodes WHERE series_item_id = $1`, seriesID); err != nil {
		return fmt.Errorf("CatalogRepo.ReplaceSeriesEpisodes delete: %w", err)
	}
	for _, e := range episodes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO series_expected_episodes (series_item_id, season, episode, title, air_date)
			 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
			seriesID, e.Season, e.Episode, e.Title, e.AirDate); err != nil {
			return fmt.Errorf("CatalogRepo.ReplaceSeriesEpisodes insert: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO series_catalog_checks (series_item_id, checked_at) VALUES ($1, $2)
		 ON CONFLICT (series_item_id) DO UPDATE SET checked_at = EXCLUDED.checked_at`, seriesID, checkedAt); err != nil {
		return fmt.Errorf("CatalogRepo.ReplaceSeriesEpisodes check: %w", err)
	}
	return tx.Commit()
}

func (r *CatalogRepo) CollectionsToCheck(ctx context.Context, olderThan time.Time, limit int) ([]library.CollectionRef, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.CollectionsToCheck")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT c.id, c.tmdb_id, c.name
		   FROM collections c
		   LEFT JOIN collection_catalog_checks k ON k.collection_id = c.id
		  WHERE c.tmdb_id IS NOT NULL
		    AND (k.checked_at IS NULL OR k.checked_at < $1)
		  ORDER BY k.checked_at NULLS FIRST, c.id
		  LIMIT $2`, olderThan, limit)
	if err != nil {
		return nil, fmt.Errorf("CatalogRepo.CollectionsToCheck: %w", err)
	}
	defer rows.Close()
	var out []library.CollectionRef
	for rows.Next() {
		var c library.CollectionRef
		if err := rows.Scan(&c.ID, &c.TmdbID, &c.Name); err != nil {
			return nil, fmt.Errorf("CatalogRepo.CollectionsToCheck scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CatalogRepo) ReplaceCollectionParts(ctx context.Context, collectionID string, parts []library.CollectionPart, checkedAt time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.ReplaceCollectionParts")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("CatalogRepo.ReplaceCollectionParts begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM collection_parts WHERE collection_id = $1`, collectionID); err != nil {
		return fmt.Errorf("CatalogRepo.ReplaceCollectionParts delete: %w", err)
	}
	for _, p := range parts {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO collection_parts (collection_id, tmdb_id, title, release_date, poster_url)
			 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
			collectionID, p.TmdbID, p.Title, p.ReleaseDate, p.PosterURL); err != nil {
			return fmt.Errorf("CatalogRepo.ReplaceCollectionParts insert: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO collection_catalog_checks (collection_id, checked_at) VALUES ($1, $2)
		 ON CONFLICT (collection_id) DO UPDATE SET checked_at = EXCLUDED.checked_at`, collectionID, checkedAt); err != nil {
		return fmt.Errorf("CatalogRepo.ReplaceCollectionParts check: %w", err)
	}
	return tx.Commit()
}

func (r *CatalogRepo) ListSeries(ctx context.Context, libraryIDs []shared.LibraryID) ([]library.SeriesRef, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.ListSeries")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mi.id, COALESCE(mc.provider_ids->>'tmdb', ''), mi.title, COALESCE(mc.year, 0), mi.library_id
		   FROM media_items mi
		   LEFT JOIN media_catalog mc ON mc.media_item_id = mi.id
		  WHERE mi.type = 'series'
		    AND (cardinality($1::text[]) = 0 OR mi.library_id = ANY($1))
		  ORDER BY mi.sort_title, mi.id`, pq.Array(libraryIDStrings(libraryIDs)))
	if err != nil {
		return nil, fmt.Errorf("CatalogRepo.ListSeries: %w", err)
	}
	defer rows.Close()
	var out []library.SeriesRef
	for rows.Next() {
		var s library.SeriesRef
		var lib string
		if err := rows.Scan(&s.ItemID, &s.TmdbID, &s.Title, &s.Year, &lib); err != nil {
			return nil, fmt.Errorf("CatalogRepo.ListSeries scan: %w", err)
		}
		s.Library = shared.LibraryID(lib)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *CatalogRepo) ExpectedEpisodes(ctx context.Context, seriesIDs []string) (map[string][]library.ExpectedEpisode, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.ExpectedEpisodes")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT series_item_id, season, episode, title, air_date
		   FROM series_expected_episodes WHERE series_item_id = ANY($1)
		  ORDER BY series_item_id, season, episode`, pq.Array(seriesIDs))
	if err != nil {
		return nil, fmt.Errorf("CatalogRepo.ExpectedEpisodes: %w", err)
	}
	defer rows.Close()
	out := map[string][]library.ExpectedEpisode{}
	for rows.Next() {
		var id string
		var e library.ExpectedEpisode
		var air sql.NullTime
		if err := rows.Scan(&id, &e.Season, &e.Episode, &e.Title, &air); err != nil {
			return nil, fmt.Errorf("CatalogRepo.ExpectedEpisodes scan: %w", err)
		}
		if air.Valid {
			t := air.Time
			e.AirDate = &t
		}
		out[id] = append(out[id], e)
	}
	return out, rows.Err()
}

// OwnedEpisodes reads the numbered episodes of each series: an episode's
// season is its parent's number. Episodes or seasons without a number are
// left out, they cannot fill a numbered gap.
func (r *CatalogRepo) OwnedEpisodes(ctx context.Context, seriesIDs []string) (map[string][]library.OwnedEpisode, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.OwnedEpisodes")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT s.parent_id, s.index_number, e.index_number
		   FROM media_items e
		   JOIN media_items s ON s.id = e.parent_id AND s.type = 'season'
		  WHERE e.type = 'episode'
		    AND s.parent_id = ANY($1)
		    AND s.index_number IS NOT NULL AND e.index_number IS NOT NULL`, pq.Array(seriesIDs))
	if err != nil {
		return nil, fmt.Errorf("CatalogRepo.OwnedEpisodes: %w", err)
	}
	defer rows.Close()
	out := map[string][]library.OwnedEpisode{}
	for rows.Next() {
		var o library.OwnedEpisode
		if err := rows.Scan(&o.SeriesID, &o.Season, &o.Episode); err != nil {
			return nil, fmt.Errorf("CatalogRepo.OwnedEpisodes scan: %w", err)
		}
		out[o.SeriesID] = append(out[o.SeriesID], o)
	}
	return out, rows.Err()
}

func (r *CatalogRepo) SagasOwned(ctx context.Context, libraryIDs []shared.LibraryID) ([]library.CollectionRef, map[string][]int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.SagasOwned")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT c.id, c.tmdb_id, c.name, mc.provider_ids->>'tmdb'
		   FROM collections c
		   JOIN collection_items ci ON ci.collection_id = c.id
		   JOIN media_items mi ON mi.id = ci.media_item_id AND mi.type = 'movie'
		   JOIN media_catalog mc ON mc.media_item_id = mi.id
		  WHERE c.tmdb_id IS NOT NULL
		    AND COALESCE(mc.provider_ids->>'tmdb', '') ~ '^[0-9]+$'
		    AND (cardinality($1::text[]) = 0 OR mi.library_id = ANY($1))
		  ORDER BY c.name, c.id`, pq.Array(libraryIDStrings(libraryIDs)))
	if err != nil {
		return nil, nil, fmt.Errorf("CatalogRepo.SagasOwned: %w", err)
	}
	defer rows.Close()
	var sagas []library.CollectionRef
	owned := map[string][]int{}
	seen := map[string]bool{}
	for rows.Next() {
		var c library.CollectionRef
		var tmdb int
		if err := rows.Scan(&c.ID, &c.TmdbID, &c.Name, &tmdb); err != nil {
			return nil, nil, fmt.Errorf("CatalogRepo.SagasOwned scan: %w", err)
		}
		if !seen[c.ID] {
			seen[c.ID] = true
			sagas = append(sagas, c)
		}
		owned[c.ID] = append(owned[c.ID], tmdb)
	}
	return sagas, owned, rows.Err()
}

func (r *CatalogRepo) CollectionParts(ctx context.Context, collectionIDs []string) (map[string][]library.CollectionPart, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.catalog.CollectionParts")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT collection_id, tmdb_id, title, release_date, poster_url
		   FROM collection_parts WHERE collection_id = ANY($1)
		  ORDER BY collection_id, release_date NULLS LAST, tmdb_id`, pq.Array(collectionIDs))
	if err != nil {
		return nil, fmt.Errorf("CatalogRepo.CollectionParts: %w", err)
	}
	defer rows.Close()
	out := map[string][]library.CollectionPart{}
	for rows.Next() {
		var id string
		var p library.CollectionPart
		var rel sql.NullTime
		if err := rows.Scan(&id, &p.TmdbID, &p.Title, &rel, &p.PosterURL); err != nil {
			return nil, fmt.Errorf("CatalogRepo.CollectionParts scan: %w", err)
		}
		if rel.Valid {
			t := rel.Time
			p.ReleaseDate = &t
		}
		out[id] = append(out[id], p)
	}
	return out, rows.Err()
}
