package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/playlist"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type PlaylistRepository struct {
	db *sql.DB
}

func NewPlaylistRepository(db *sql.DB) *PlaylistRepository {
	return &PlaylistRepository{db: db}
}

func (r *PlaylistRepository) Create(ctx context.Context, pl *playlist.Playlist) error {
	rulesJSON, err := json.Marshal(pl.Rules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO playlists (id, name, description, user_id, type, rules, sort_by, sort_order, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		pl.ID, pl.Name, pl.Description, pl.UserID, pl.Type, rulesJSON, pl.SortBy, pl.SortOrder, pl.CreatedAt, pl.UpdatedAt,
	)
	return err
}

func (r *PlaylistRepository) GetByID(ctx context.Context, id string) (*playlist.Playlist, error) {
	pl := &playlist.Playlist{}
	var rulesJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, user_id, type, rules, sort_by, sort_order, created_at, updated_at
		 FROM playlists WHERE id = $1`, id,
	).Scan(&pl.ID, &pl.Name, &pl.Description, &pl.UserID, &pl.Type, &rulesJSON, &pl.SortBy, &pl.SortOrder, &pl.CreatedAt, &pl.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rulesJSON, &pl.Rules); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
	}
	return pl, nil
}

func (r *PlaylistRepository) List(ctx context.Context, userID string) ([]*playlist.Playlist, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, user_id, type, rules, sort_by, sort_order, created_at, updated_at
		 FROM playlists WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*playlist.Playlist
	for rows.Next() {
		pl := &playlist.Playlist{}
		var rulesJSON []byte
		if err := rows.Scan(&pl.ID, &pl.Name, &pl.Description, &pl.UserID, &pl.Type, &rulesJSON, &pl.SortBy, &pl.SortOrder, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rulesJSON, &pl.Rules); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
		}
		result = append(result, pl)
	}
	return result, rows.Err()
}

func (r *PlaylistRepository) Update(ctx context.Context, pl *playlist.Playlist) error {
	rulesJSON, err := json.Marshal(pl.Rules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE playlists SET name=$1, description=$2, rules=$3, sort_by=$4, sort_order=$5, updated_at=$6 WHERE id=$7`,
		pl.Name, pl.Description, rulesJSON, pl.SortBy, pl.SortOrder, pl.UpdatedAt, pl.ID,
	)
	return err
}

func (r *PlaylistRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM playlists WHERE id = $1`, id)
	return err
}

// PlaylistItemRepository

type PlaylistItemRepository struct {
	db *sql.DB
}

func NewPlaylistItemRepository(db *sql.DB) *PlaylistItemRepository {
	return &PlaylistItemRepository{db: db}
}

func (r *PlaylistItemRepository) Add(ctx context.Context, item *playlist.PlaylistItem) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO playlist_items (playlist_id, media_item_id, sort_order, added_at)
		 VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
		item.PlaylistID, item.MediaItemID, item.SortOrder, item.AddedAt,
	)
	return err
}

func (r *PlaylistItemRepository) Remove(ctx context.Context, playlistID, mediaItemID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM playlist_items WHERE playlist_id = $1 AND media_item_id = $2`,
		playlistID, mediaItemID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	if n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *PlaylistItemRepository) GetByPlaylistID(ctx context.Context, playlistID string) ([]*playlist.PlaylistItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT playlist_id, media_item_id, sort_order, added_at
		 FROM playlist_items WHERE playlist_id = $1 ORDER BY sort_order`, playlistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*playlist.PlaylistItem
	for rows.Next() {
		item := &playlist.PlaylistItem{}
		if err := rows.Scan(&item.PlaylistID, &item.MediaItemID, &item.SortOrder, &item.AddedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PlaylistItemRepository) Reorder(ctx context.Context, playlistID string, mediaItemIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(mediaItemIDs) == 0 {
		return tx.Commit()
	}
	var vals []string
	args := []any{playlistID}
	for i, mid := range mediaItemIDs {
		args = append(args, mid, i)
		vals = append(vals, fmt.Sprintf("($%d, $%d)", len(args)-1, len(args)))
	}
	query := `UPDATE playlist_items SET sort_order = v.new_order::int
		FROM (VALUES ` + strings.Join(vals, ", ") + `) AS v(mid, new_order)
		WHERE playlist_items.playlist_id = $1 AND playlist_items.media_item_id = v.mid`
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return tx.Commit()
}

// SmartPlaylistQuerier implements rule-based SQL queries

type SmartPlaylistQuerier struct {
	db *sql.DB
}

func NewSmartPlaylistQuerier(db *sql.DB) *SmartPlaylistQuerier {
	return &SmartPlaylistQuerier{db: db}
}

func (q *SmartPlaylistQuerier) QueryByRules(ctx context.Context, rules []playlist.Rule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error) {
	query := `SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		file_path, file_size, duration_ms, bitrate, container_format, index_number, community_rating,
		official_rating, premiere_date, year, provider_ids, genres, studios, tags, added_at, updated_at
		FROM media_items`

	var conditions []string
	var args []any
	argIdx := 1

	for _, rule := range rules {
		cond, ruleArgs := buildRuleCondition(rule, &argIdx)
		if cond != "" {
			conditions = append(conditions, cond)
			args = append(args, ruleArgs...)
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	validSort := map[string]string{
		"title":    "sort_title",
		"year":     "year",
		"added_at": "added_at",
		"rating":   "community_rating",
	}
	col, ok := validSort[sortBy]
	if !ok {
		col = "added_at"
	}
	dir := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		dir = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s NULLS LAST", col, dir)

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMediaItems(rows)
}

func buildRuleCondition(rule playlist.Rule, argIdx *int) (string, []any) {
	switch rule.Field {
	case "genre":
		val, err := json.Marshal([]string{rule.Value})
		if err != nil {
			slog.Warn("marshal genre rule", slog.Any("err", err))
			return "", nil
		}
		cond := fmt.Sprintf("genres @> $%d::jsonb", *argIdx)
		*argIdx++
		return cond, []any{string(val)}
	case "type":
		cond := fmt.Sprintf("type = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "year":
		switch rule.Operator {
		case "gt":
			cond := fmt.Sprintf("year > $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case "lt":
			cond := fmt.Sprintf("year < $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case "gte":
			cond := fmt.Sprintf("year >= $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case "lte":
			cond := fmt.Sprintf("year <= $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		default:
			cond := fmt.Sprintf("year = $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		}
	case "library_id":
		cond := fmt.Sprintf("library_id = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "rating_gte":
		cond := fmt.Sprintf("community_rating >= $%d::double precision", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	default:
		return "", nil
	}
}

func scanMediaItems(rows *sql.Rows) ([]*library.MediaItem, error) {
	var result []*library.MediaItem
	for rows.Next() {
		item := &library.MediaItem{}
		var providerIDsJSON, genresJSON, studiosJSON, tagsJSON []byte
		var filePath sql.NullString
		var parentID sql.NullString
		err := rows.Scan(
			&item.ID, &item.LibraryID, &parentID, &item.Type,
			&item.Title, &item.SortTitle, &item.OriginalTitle, &item.Overview, &item.Tagline,
			&filePath, &item.FileSize, &item.DurationMs, &item.Bitrate, &item.ContainerFormat,
			&item.IndexNumber, &item.CommunityRating, &item.OfficialRating,
			&item.PremiereDate, &item.Year, &providerIDsJSON, &genresJSON, &studiosJSON, &tagsJSON,
			&item.AddedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if parentID.Valid {
			item.ParentID = parentID.String
		}
		if filePath.Valid {
			item.FilePath = filePath.String
		}
		if err := json.Unmarshal(providerIDsJSON, &item.ProviderIDs); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "provider_ids"), slog.Any("err", err))
		}
		if err := json.Unmarshal(genresJSON, &item.Genres); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "genres"), slog.Any("err", err))
		}
		if err := json.Unmarshal(studiosJSON, &item.Studios); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "studios"), slog.Any("err", err))
		}
		if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "tags"), slog.Any("err", err))
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Ensure interface compliance
var _ playlist.PlaylistRepository = (*PlaylistRepository)(nil)
var _ playlist.PlaylistItemRepository = (*PlaylistItemRepository)(nil)
var _ appplaylist.SmartPlaylistQuerier = (*SmartPlaylistQuerier)(nil)
