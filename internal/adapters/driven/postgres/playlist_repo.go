package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type PlaylistRepository struct {
	db *sql.DB
}

func NewPlaylistRepository(db *sql.DB) *PlaylistRepository {
	return &PlaylistRepository{db: db}
}

func (r *PlaylistRepository) Create(ctx context.Context, pl *playlist.Playlist) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist.Create")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", pl.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rulesJSON, err := json.Marshal(pl.Rules)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistRepo.Create marshal rules: %w", err)
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO playlists (id, name, description, profile_id, type, rules, sort_by, sort_order, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		pl.ID, pl.Name, pl.Description, pl.ProfileID, pl.Type, rulesJSON, pl.SortBy, pl.SortOrder, pl.CreatedAt, pl.UpdatedAt,
	)
	if err != nil {
		if isPlaylistNameConflict(err) {
			return playlist.ErrDuplicateName
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistRepo.Create: %w", err)
	}
	return nil
}

// isPlaylistNameConflict matches the unique (profile_id, name) index
// (migration 000148) so a double-tapped Create or a rename onto a taken name
// surfaces as the domain sentinel instead of a raw pq error.
func isPlaylistNameConflict(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505" && pqErr.Constraint == "idx_playlists_profile_name"
}

func (r *PlaylistRepository) GetByID(ctx context.Context, id string) (*playlist.Playlist, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	pl := &playlist.Playlist{}
	var rulesJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, description, profile_id, type, rules, sort_by, sort_order, created_at, updated_at
		 FROM playlists WHERE id = $1`, id,
	).Scan(&pl.ID, &pl.Name, &pl.Description, &pl.ProfileID, &pl.Type, &rulesJSON, &pl.SortBy, &pl.SortOrder, &pl.CreatedAt, &pl.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaylistRepo.GetByID: %w", err)
	}
	if err := json.Unmarshal(rulesJSON, &pl.Rules); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
	}
	return pl, nil
}

func (r *PlaylistRepository) List(ctx context.Context, profileID string) ([]*playlist.Playlist, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist.List")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, name, description, profile_id, type, rules, sort_by, sort_order, created_at, updated_at
		 FROM playlists WHERE profile_id = $1 ORDER BY created_at DESC LIMIT 1000`, profileID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaylistRepo.List: %w", err)
	}
	defer rows.Close()

	var result []*playlist.Playlist
	for rows.Next() {
		pl := &playlist.Playlist{}
		var rulesJSON []byte
		if err := rows.Scan(&pl.ID, &pl.Name, &pl.Description, &pl.ProfileID, &pl.Type, &rulesJSON, &pl.SortBy, &pl.SortOrder, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("PlaylistRepo.List scan: %w", err)
		}
		if err := json.Unmarshal(rulesJSON, &pl.Rules); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
		}
		result = append(result, pl)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaylistRepo.List rows: %w", err)
	}
	return result, nil
}

func (r *PlaylistRepository) Update(ctx context.Context, pl *playlist.Playlist) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist.Update")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", pl.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rulesJSON, err := json.Marshal(pl.Rules)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistRepo.Update marshal rules: %w", err)
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE playlists SET name=$1, description=$2, rules=$3, sort_by=$4, sort_order=$5, updated_at=$6 WHERE id=$7`,
		pl.Name, pl.Description, rulesJSON, pl.SortBy, pl.SortOrder, pl.UpdatedAt, pl.ID,
	)
	if err != nil {
		if isPlaylistNameConflict(err) {
			return playlist.ErrDuplicateName
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistRepo.Update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *PlaylistRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM playlists WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistRepo.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
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
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist_item.Add")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", string(item.PlaylistID)), attribute.String("media_item.id", string(item.MediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	mediaType := item.MediaType
	if mediaType == "" {
		mediaType = playlist.MediaTypeVideo
	}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO playlist_items (playlist_id, media_item_id, sort_order, added_at, media_type)
		 VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
		item.PlaylistID, item.MediaItemID, item.SortOrder, item.AddedAt, mediaType,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistItemRepo.Add: %w", err)
	}
	return nil
}

func (r *PlaylistItemRepository) AddNextOrder(ctx context.Context, item *playlist.PlaylistItem, maxItems int) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist_item.AddNextOrder")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", item.PlaylistID), attribute.String("media_item.id", string(item.MediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	mediaType := item.MediaType
	if mediaType == "" {
		mediaType = playlist.MediaTypeVideo
	}
	// Compute sort_order inside the INSERT (MAX+1) so two concurrent adds can't
	// both read the same max in Go and write a colliding order. Aggregate over an
	// empty playlist yields one row with MAX=NULL → order 0 for the first item.
	// The item cap rides INSIDE the statement (HAVING). It NARROWS the old
	// Go-side read-then-insert race to the statement window - under READ
	// COMMITTED two simultaneous statements at cap-1 can still both pass
	// (cap+1), which is accepted: the cap is an anti-abuse bound, not an
	// invariant, and closing it fully would need an advisory lock + TxUnit
	// on a hot path. cap <= 0 = uncapped.
	cap64 := int64(maxItems)
	if maxItems <= 0 {
		cap64 = int64(^uint64(0) >> 1)
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO playlist_items (playlist_id, media_item_id, sort_order, added_at, media_type)
		 SELECT $1, $2, COALESCE(MAX(sort_order), -1) + 1, $3, $4
		 FROM playlist_items WHERE playlist_id = $1
		 HAVING COUNT(*) < $5
		 ON CONFLICT DO NOTHING`,
		item.PlaylistID, item.MediaItemID, item.AddedAt, mediaType, cap64,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistItemRepo.AddNextOrder: %w", err)
	}
	if n, raErr := res.RowsAffected(); raErr == nil && n == 0 {
		// Zero rows is EITHER the duplicate no-op (conflict) or the cap. Only
		// the count says which; the extra query only runs on this cold path.
		var count int64
		if cErr := txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT COUNT(*) FROM playlist_items WHERE playlist_id = $1`, item.PlaylistID,
		).Scan(&count); cErr == nil && count >= cap64 {
			return playlist.ErrMaxItemsReached
		}
	}
	return nil
}

func (r *PlaylistItemRepository) Remove(ctx context.Context, playlistID, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist_item.Remove")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", playlistID), attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM playlist_items WHERE playlist_id = $1 AND media_item_id = $2`,
		playlistID, mediaItemID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistItemRepo.Remove: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	if n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *PlaylistItemRepository) GetByPlaylistID(ctx context.Context, playlistID string) ([]*playlist.PlaylistItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist_item.GetByPlaylistID")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", playlistID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT playlist_id, media_item_id, sort_order, added_at, media_type
		 FROM playlist_items WHERE playlist_id = $1 ORDER BY sort_order`, playlistID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistID: %w", err)
	}
	defer rows.Close()

	var result []*playlist.PlaylistItem
	for rows.Next() {
		item := &playlist.PlaylistItem{}
		if err := rows.Scan(&item.PlaylistID, &item.MediaItemID, &item.SortOrder, &item.AddedAt, &item.MediaType); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistID scan: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistID rows: %w", err)
	}
	return result, nil
}

// GetByPlaylistIDs batches the per-playlist lookup into one query - the
// playlist generator's de-duplication scan used to call GetByPlaylistID once
// per profile playlist (N+1 round trips per generation).
func (r *PlaylistItemRepository) GetByPlaylistIDs(ctx context.Context, playlistIDs []string) (map[string][]*playlist.PlaylistItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist_item.GetByPlaylistIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("playlist.count", len(playlistIDs)))
	if len(playlistIDs) == 0 {
		return map[string][]*playlist.PlaylistItem{}, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT playlist_id, media_item_id, sort_order, added_at, media_type
		 FROM playlist_items WHERE playlist_id = ANY($1) ORDER BY playlist_id, sort_order`, pq.Array(playlistIDs),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistIDs: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]*playlist.PlaylistItem)
	for rows.Next() {
		item := &playlist.PlaylistItem{}
		if err := rows.Scan(&item.PlaylistID, &item.MediaItemID, &item.SortOrder, &item.AddedAt, &item.MediaType); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistIDs scan: %w", err)
		}
		result[item.PlaylistID] = append(result[item.PlaylistID], item)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaylistItemRepo.GetByPlaylistIDs rows: %w", err)
	}
	return result, nil
}

func (r *PlaylistItemRepository) Reorder(ctx context.Context, playlistID string, mediaItemIDs []string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist_item.Reorder")
	defer span.End()
	span.SetAttributes(attribute.String("playlist.id", playlistID), attribute.Int("media_item.count", len(mediaItemIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistItemRepo.Reorder begin tx: %w", err)
	}
	defer tx.Rollback()

	if len(mediaItemIDs) == 0 {
		if err := tx.Commit(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		return nil
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
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaylistItemRepo.Reorder exec: %w", err)
	}
	// The membership pre-check in the service runs OUTSIDE this transaction:
	// an item removed in between would silently half-apply the reorder.
	// Fewer matched rows than distinct requested ids = drift, roll back.
	if n, aerr := res.RowsAffected(); aerr == nil {
		distinct := make(map[string]struct{}, len(mediaItemIDs))
		for _, mid := range mediaItemIDs {
			distinct[mid] = struct{}{}
		}
		if int(n) != len(distinct) {
			span.SetStatus(codes.Error, playlist.ErrReorderMembershipDrift.Error())
			return playlist.ErrReorderMembershipDrift
		}
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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

func (q *SmartPlaylistQuerier) Query(ctx context.Context, spec playlist.Spec, sortBy, sortOrder string, limit int) ([]shared.MediaItemID, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.smart_playlist.Query")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	query := "SELECT mi.id " + mediaItemFromJoins

	var args []any
	argIdx := 1
	if clause, specArgs := playlistSpecSQL(spec, &argIdx); clause != "" {
		query += " WHERE " + clause
		args = specArgs
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

	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("SmartPlaylistQuerier.Query: %w", err)
	}
	defer rows.Close()

	var ids []shared.MediaItemID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("SmartPlaylistQuerier.Query scan: %w", err)
		}
		ids = append(ids, shared.MediaItemID(id))
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("SmartPlaylistQuerier.Query rows: %w", err)
	}
	return ids, nil
}

// Ensure interface compliance
var _ playlist.PlaylistRepository = (*PlaylistRepository)(nil)
var _ playlist.PlaylistItemRepository = (*PlaylistItemRepository)(nil)
var _ playlist.SmartPlaylistQuerier = (*SmartPlaylistQuerier)(nil)

// PurgeOrphans deletes the items left pointing at a removed media item or
// music track. Set-based, so a library deletion that cascades thousands of
// media away costs one statement.
func (r *PlaylistItemRepository) PurgeOrphans(ctx context.Context) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playlist_item.PurgeOrphans")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM playlist_items pi
		 WHERE (pi.media_type = $1 AND NOT EXISTS (SELECT 1 FROM music_tracks t WHERE t.id = pi.media_item_id))
		    OR (pi.media_type <> $1 AND NOT EXISTS (SELECT 1 FROM media_items m WHERE m.id = pi.media_item_id))`,
		playlist.MediaTypeMusic,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("PlaylistItemRepo.PurgeOrphans: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
