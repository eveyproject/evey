package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
)

type PlaylistRepository struct {
	db *sql.DB
}

func NewPlaylistRepository(db *sql.DB) *PlaylistRepository {
	return &PlaylistRepository{db: db}
}

func (r *PlaylistRepository) Create(ctx context.Context, pl *playlist.Playlist) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rulesJSON, err := json.Marshal(pl.Rules)
	if err != nil {
		return fmt.Errorf("PlaylistRepo.Create marshal rules: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO playlists (id, name, description, user_id, type, rules, sort_by, sort_order, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		pl.ID, pl.Name, pl.Description, pl.UserID, pl.Type, rulesJSON, pl.SortBy, pl.SortOrder, pl.CreatedAt, pl.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("PlaylistRepo.Create: %w", err)
	}
	return nil
}

func (r *PlaylistRepository) GetByID(ctx context.Context, id string) (*playlist.Playlist, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
		return nil, fmt.Errorf("PlaylistRepo.GetByID: %w", err)
	}
	if err := json.Unmarshal(rulesJSON, &pl.Rules); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
	}
	return pl, nil
}

func (r *PlaylistRepository) List(ctx context.Context, userID string) ([]*playlist.Playlist, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, user_id, type, rules, sort_by, sort_order, created_at, updated_at
		 FROM playlists WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1000`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("PlaylistRepo.List: %w", err)
	}
	defer rows.Close()

	var result []*playlist.Playlist
	for rows.Next() {
		pl := &playlist.Playlist{}
		var rulesJSON []byte
		if err := rows.Scan(&pl.ID, &pl.Name, &pl.Description, &pl.UserID, &pl.Type, &rulesJSON, &pl.SortBy, &pl.SortOrder, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
			return nil, fmt.Errorf("PlaylistRepo.List scan: %w", err)
		}
		if err := json.Unmarshal(rulesJSON, &pl.Rules); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
		}
		result = append(result, pl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PlaylistRepo.List rows: %w", err)
	}
	return result, nil
}

func (r *PlaylistRepository) Update(ctx context.Context, pl *playlist.Playlist) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rulesJSON, err := json.Marshal(pl.Rules)
	if err != nil {
		return fmt.Errorf("PlaylistRepo.Update marshal rules: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE playlists SET name=$1, description=$2, rules=$3, sort_by=$4, sort_order=$5, updated_at=$6 WHERE id=$7`,
		pl.Name, pl.Description, rulesJSON, pl.SortBy, pl.SortOrder, pl.UpdatedAt, pl.ID,
	)
	if err != nil {
		return fmt.Errorf("PlaylistRepo.Update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *PlaylistRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `DELETE FROM playlists WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("PlaylistRepo.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// PlaylistItemRepository

type PlaylistItemRepository struct {
	db *sql.DB
}

func NewPlaylistItemRepository(db *sql.DB) *PlaylistItemRepository {
	return &PlaylistItemRepository{db: db}
}

func (r *PlaylistItemRepository) Add(ctx context.Context, item *playlist.PlaylistItem) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	mediaType := item.MediaType
	if mediaType == "" {
		mediaType = playlist.MediaTypeVideo
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO playlist_items (playlist_id, media_item_id, sort_order, added_at, media_type)
		 VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
		item.PlaylistID, item.MediaItemID, item.SortOrder, item.AddedAt, mediaType,
	)
	if err != nil {
		return fmt.Errorf("PlaylistItemRepo.Add: %w", err)
	}
	return nil
}

func (r *PlaylistItemRepository) Remove(ctx context.Context, playlistID, mediaItemID string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM playlist_items WHERE playlist_id = $1 AND media_item_id = $2`,
		playlistID, mediaItemID,
	)
	if err != nil {
		return fmt.Errorf("PlaylistItemRepo.Remove: %w", err)
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
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT playlist_id, media_item_id, sort_order, added_at, media_type
		 FROM playlist_items WHERE playlist_id = $1 ORDER BY sort_order`, playlistID,
	)
	if err != nil {
		return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistID: %w", err)
	}
	defer rows.Close()

	var result []*playlist.PlaylistItem
	for rows.Next() {
		item := &playlist.PlaylistItem{}
		if err := rows.Scan(&item.PlaylistID, &item.MediaItemID, &item.SortOrder, &item.AddedAt, &item.MediaType); err != nil {
			return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistID scan: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistID rows: %w", err)
	}
	return result, nil
}

func (r *PlaylistItemRepository) Reorder(ctx context.Context, playlistID string, mediaItemIDs []string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("PlaylistItemRepo.Reorder begin tx: %w", err)
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
		return fmt.Errorf("PlaylistItemRepo.Reorder exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("PlaylistItemRepo.Reorder commit: %w", err)
	}
	return nil
}

// SmartPlaylistQuerier implements rule-based SQL queries

type SmartPlaylistQuerier struct {
	db *sql.DB
}

func NewSmartPlaylistQuerier(db *sql.DB) *SmartPlaylistQuerier {
	return &SmartPlaylistQuerier{db: db}
}

func (q *SmartPlaylistQuerier) QueryByRules(ctx context.Context, rules []playlist.Rule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	query := fmt.Sprintf("SELECT %s %s", mediaItemColumns, mediaItemFromJoins)

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
		"title":    "mi.sort_title",
		"year":     "mc.year",
		"added_at": "mi.added_at",
		"rating":   "mcl.community_rating",
	}
	col, ok := validSort[sortBy]
	if !ok {
		col = "mi.added_at"
	}
	dir := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		dir = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s NULLS LAST", col, dir)

	if limit <= 0 || limit > DefaultListLimit {
		limit = DefaultListLimit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("SmartPlaylistQuerier.QueryByRules: %w", err)
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
		cond := fmt.Sprintf("mc.genres @> $%d::jsonb", *argIdx)
		*argIdx++
		return cond, []any{string(val)}
	case "type":
		cond := fmt.Sprintf("mi.type = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "year":
		op := "="
		switch rule.Operator {
		case "gt":
			op = ">"
		case "lt":
			op = "<"
		case "gte":
			op = ">="
		case "lte":
			op = "<="
		}
		cond := fmt.Sprintf("mc.year %s $%d", op, *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "library_id":
		cond := fmt.Sprintf("mi.library_id = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "rating_gte":
		cond := fmt.Sprintf("mcl.community_rating >= $%d::double precision", *argIdx)
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
		if err := scanMediaItemFields(rows, item); err != nil {
			return nil, fmt.Errorf("scanMediaItems: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanMediaItems rows: %w", err)
	}
	return result, nil
}

// Ensure interface compliance
var _ playlist.PlaylistRepository = (*PlaylistRepository)(nil)
var _ playlist.PlaylistItemRepository = (*PlaylistItemRepository)(nil)
var _ playlist.SmartPlaylistQuerier = (*SmartPlaylistQuerier)(nil)
