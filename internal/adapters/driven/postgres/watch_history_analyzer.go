package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// WatchHistoryAnalyzer implements notification.WatchHistoryAnalyzer using PostgreSQL.
type WatchHistoryAnalyzer struct {
	db *sql.DB
}

func NewWatchHistoryAnalyzer(db *sql.DB) *WatchHistoryAnalyzer {
	return &WatchHistoryAnalyzer{db: db}
}

// favoriteGenresSessionWindow bounds how far back the taste profile looks, in
// sessions per profile. Shared by both query paths on purpose: they drifted
// apart once - the single-profile one capped at 200 while the batch one joined
// the WHOLE of play_sessions per profile - and the retention purge for that
// table is OFF by default (EVEY_PLAY_SESSION_RETENTION_DAYS=0), so nothing
// upstream bounded it either. On a household with a couple of years of history
// that is the difference between a fixed cost and one that grows forever.
const favoriteGenresSessionWindow = 200

// GetFavoriteGenres returns genres the profile has watched at least minCount times.
func (a *WatchHistoryAnalyzer) GetFavoriteGenres(ctx context.Context, profileID shared.ProfileID, minCount int) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchhistory.GetFavoriteGenres")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, a.db).QueryContext(ctx, `
		WITH recent_sessions AS (
			SELECT media_item_id
			FROM play_sessions
			WHERE profile_id = $1
			ORDER BY started_at DESC
			LIMIT $3
		)
		SELECT genre, COUNT(*) AS cnt
		FROM recent_sessions rs
		INNER JOIN media_catalog mc ON mc.media_item_id = rs.media_item_id
		CROSS JOIN jsonb_array_elements_text(
			CASE WHEN jsonb_typeof(mc.genres) = 'array' THEN mc.genres ELSE '[]'::jsonb END
		) AS genre
		GROUP BY genre
		HAVING COUNT(*) >= $2
		ORDER BY cnt DESC
	`, string(profileID), minCount, favoriteGenresSessionWindow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var genres []string
	for rows.Next() {
		var genre string
		var cnt int
		if err := rows.Scan(&genre, &cnt); err != nil {
			return nil, err
		}
		genres = append(genres, genre)
	}
	return genres, rows.Err()
}

// GetFavoriteGenresBatch returns favorite genres for multiple profiles in a single query.
func (a *WatchHistoryAnalyzer) GetFavoriteGenresBatch(ctx context.Context, profileIDs []shared.ProfileID, minCount int) (map[shared.ProfileID][]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchhistory.GetFavoriteGenresBatch")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(profileIDs) == 0 {
		return map[shared.ProfileID][]string{}, nil
	}

	args := make([]any, 0, len(profileIDs)+1)
	placeholders := make([]string, len(profileIDs))
	for i, pid := range profileIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, string(pid))
	}
	args = append(args, minCount)

	// ROW_NUMBER per profile is the batch equivalent of the single-profile
	// LIMIT: same window, same answer, one query for N profiles.
	args = append(args, favoriteGenresSessionWindow)
	query := fmt.Sprintf(`
		WITH recent_sessions AS (
			SELECT profile_id, media_item_id,
			       ROW_NUMBER() OVER (PARTITION BY profile_id ORDER BY started_at DESC) AS rn
			FROM play_sessions
			WHERE profile_id IN (%s)
		)
		SELECT rs.profile_id, genre, COUNT(*) AS cnt
		FROM recent_sessions rs
		INNER JOIN media_catalog mc ON mc.media_item_id = rs.media_item_id
		CROSS JOIN jsonb_array_elements_text(
			CASE WHEN jsonb_typeof(mc.genres) = 'array' THEN mc.genres ELSE '[]'::jsonb END
		) AS genre
		WHERE rs.rn <= $%d
		GROUP BY rs.profile_id, genre
		HAVING COUNT(*) >= $%d
		ORDER BY rs.profile_id, cnt DESC
	`, strings.Join(placeholders, ","), len(profileIDs)+2, len(profileIDs)+1)

	rows, err := txFromCtx(ctx, a.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[shared.ProfileID][]string, len(profileIDs))
	for rows.Next() {
		var profileID, genre string
		var cnt int
		if err := rows.Scan(&profileID, &genre, &cnt); err != nil {
			return nil, err
		}
		result[shared.ProfileID(profileID)] = append(result[shared.ProfileID(profileID)], genre)
	}
	return result, rows.Err()
}

// HasWatchedSeries returns true if the profile has any completed episodes in the series.
func (a *WatchHistoryAnalyzer) HasWatchedSeries(ctx context.Context, profileID shared.ProfileID, mediaItemID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchhistory.HasWatchedSeries")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, a.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM watch_progress wp
		INNER JOIN media_items mi ON mi.id = wp.media_item_id
		WHERE wp.profile_id = $1
		  AND mi.parent_id IN (
		      SELECT id FROM media_items WHERE parent_id = $2 AND type = 'season'
		  )
		  AND wp.completed = TRUE
	`, string(profileID), mediaItemID).Scan(&count)
	return count > 0, err
}

// IsCurrentlyWatchingSeries returns true if the profile has an in-progress episode in the series.
func (a *WatchHistoryAnalyzer) IsCurrentlyWatchingSeries(ctx context.Context, profileID shared.ProfileID, mediaItemID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchhistory.IsCurrentlyWatchingSeries")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, a.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM watch_progress wp
		INNER JOIN media_items mi ON mi.id = wp.media_item_id
		WHERE wp.profile_id = $1
		  AND mi.parent_id IN (
		      SELECT id FROM media_items WHERE parent_id = $2 AND type = 'season'
		  )
		  AND wp.position_ms > 0
		  AND wp.completed = FALSE
	`, string(profileID), mediaItemID).Scan(&count)
	return count > 0, err
}

// CalculateRelevanceScore counts how many of the profile's favorite genres match the given genres.
func (a *WatchHistoryAnalyzer) CalculateRelevanceScore(ctx context.Context, profileID shared.ProfileID, genres []string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchhistory.CalculateRelevanceScore")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(genres) == 0 {
		return 0, nil
	}
	favorites, err := a.GetFavoriteGenres(ctx, profileID, 3)
	if err != nil {
		return 0, err
	}
	favSet := make(map[string]bool, len(favorites))
	for _, g := range favorites {
		favSet[g] = true
	}
	score := 0
	for _, g := range genres {
		if favSet[g] {
			score++
		}
	}
	return score, nil
}

var _ notification.WatchHistoryAnalyzer = (*WatchHistoryAnalyzer)(nil)
