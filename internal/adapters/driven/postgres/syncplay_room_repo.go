package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/syncplay"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type SyncPlayRoomRepo struct {
	db *sql.DB
}

func NewSyncPlayRoomRepo(db *sql.DB) *SyncPlayRoomRepo {
	return &SyncPlayRoomRepo{db: db}
}

func (r *SyncPlayRoomRepo) CountActiveByHost(ctx context.Context, hostID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.CountActiveByHost")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var n int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM syncplay_rooms WHERE host_id = $1`, hostID).Scan(&n)
	if err != nil {
		span.RecordError(err)
		return 0, err
	}
	return n, nil
}

func (r *SyncPlayRoomRepo) Create(ctx context.Context, room *syncplay.Room) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.Create")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	membersJSON, err := json.Marshal(room.Members)
	if err != nil {
		return err
	}
	stateJSON, err := json.Marshal(room.State)
	if err != nil {
		return err
	}
	queueJSON, err := json.Marshal(room.Queue)
	if err != nil {
		return err
	}
	mediaType := room.MediaType
	if mediaType == "" {
		mediaType = "video"
	}
	var playlistID sql.NullString
	if room.PlaylistID != nil {
		playlistID = sql.NullString{String: *room.PlaylistID, Valid: true}
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO syncplay_rooms (id, code, name, host_id, media_id, members, state, created_at, media_type, playlist_id, queue, track_index)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		room.ID, room.Code, room.Name, room.HostID, room.MediaID, membersJSON, stateJSON, room.CreatedAt, mediaType, playlistID, queueJSON, room.TrackIndex,
	)
	return err
}

func (r *SyncPlayRoomRepo) Get(ctx context.Context, id string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.Get")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanRoom(ctx, `SELECT id, code, name, host_id, media_id, members, state, created_at, media_type, playlist_id, queue, track_index
		 FROM syncplay_rooms WHERE id = $1`, id)
}

func (r *SyncPlayRoomRepo) GetByCode(ctx context.Context, code string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.GetByCode")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanRoom(ctx, `SELECT id, code, name, host_id, media_id, members, state, created_at, media_type, playlist_id, queue, track_index
		 FROM syncplay_rooms WHERE code = $1`, code)
}

func (r *SyncPlayRoomRepo) scanRoom(ctx context.Context, query string, arg string) (*syncplay.Room, error) {
	var room syncplay.Room
	var membersJSON, stateJSON, queueJSON []byte
	var playlistID sql.NullString
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, query, arg).Scan(
		&room.ID, &room.Code, &room.Name, &room.HostID, &room.MediaID,
		&membersJSON, &stateJSON, &room.CreatedAt, &room.MediaType,
		&playlistID, &queueJSON, &room.TrackIndex,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(membersJSON, &room.Members); err != nil {
		slog.Warn("syncplay_rooms: unmarshal members", slog.Any("err", err))
	}
	if err := json.Unmarshal(stateJSON, &room.State); err != nil {
		slog.Warn("syncplay_rooms: unmarshal state", slog.Any("err", err))
	}
	if len(queueJSON) > 0 {
		if err := json.Unmarshal(queueJSON, &room.Queue); err != nil {
			slog.Warn("syncplay_rooms: unmarshal queue", slog.Any("err", err))
		}
	}
	if playlistID.Valid {
		v := playlistID.String
		room.PlaylistID = &v
	}
	return &room, nil
}

func (r *SyncPlayRoomRepo) AddMember(ctx context.Context, roomID string, member syncplay.Member) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.AddMember")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var membersJSON []byte
	err = tx.QueryRowContext(ctx,
		`SELECT members FROM syncplay_rooms WHERE id = $1 FOR UPDATE`,
		roomID,
	).Scan(&membersJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return shared.ErrNotFound
	}
	if err != nil {
		return err
	}

	var members []syncplay.Member
	if err = json.Unmarshal(membersJSON, &members); err != nil {
		return err
	}
	for _, m := range members {
		if m.UserID == member.UserID {
			return nil // idempotent: already present, no-op
		}
	}
	members = append(members, member)
	updated, err := json.Marshal(members)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE syncplay_rooms SET members = $1 WHERE id = $2`,
		updated, roomID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SyncPlayRoomRepo) RemoveMember(ctx context.Context, roomID string, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.RemoveMember")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var membersJSON []byte
	err = tx.QueryRowContext(ctx,
		`SELECT members FROM syncplay_rooms WHERE id = $1 FOR UPDATE`,
		roomID,
	).Scan(&membersJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return shared.ErrNotFound
	}
	if err != nil {
		return err
	}

	var members []syncplay.Member
	if err = json.Unmarshal(membersJSON, &members); err != nil {
		return err
	}
	filtered := make([]syncplay.Member, 0, len(members))
	for _, m := range members {
		if string(m.UserID) != userID {
			filtered = append(filtered, m)
		}
	}
	updated, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE syncplay_rooms SET members = $1 WHERE id = $2`,
		updated, roomID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveMemberAndDeleteIfEmpty removes userID from the room and, when no
// members remain, deletes the room - all under one FOR UPDATE lock.
// If the departing member was the host and the room is not deleted, the
// oldest remaining member (earliest JoinedAt; tie-break: ascending UserID) is
// elected as the new host inside the same transaction, preventing a ghost host
// from a concurrent DisconnectMember. Returns deleted=true when the room was
// removed; newHostID is non-empty when a host election occurred.
func (r *SyncPlayRoomRepo) RemoveMemberAndDeleteIfEmpty(ctx context.Context, roomID string, userID string) (bool, string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.RemoveMemberAndDeleteIfEmpty")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback()

	var membersJSON []byte
	var hostID shared.UserID
	err = tx.QueryRowContext(ctx,
		`SELECT members, host_id FROM syncplay_rooms WHERE id = $1 FOR UPDATE`,
		roomID,
	).Scan(&membersJSON, &hostID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", shared.ErrNotFound
	}
	if err != nil {
		return false, "", err
	}

	var members []syncplay.Member
	if err = json.Unmarshal(membersJSON, &members); err != nil {
		return false, "", err
	}
	filtered := make([]syncplay.Member, 0, len(members))
	for _, m := range members {
		if string(m.UserID) != userID {
			filtered = append(filtered, m)
		}
	}

	if len(filtered) == 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM syncplay_rooms WHERE id = $1`, roomID); err != nil {
			return false, "", err
		}
		if err = tx.Commit(); err != nil {
			return false, "", err
		}
		return true, "", nil
	}

	updated, err := json.Marshal(filtered)
	if err != nil {
		return false, "", err
	}

	// Elect a new host when the departing member was the host. The election
	// runs under the same FOR UPDATE lock so a concurrent DisconnectMember sees
	// the committed result and cannot elect a member who is already gone.
	newHostID := ""
	if string(hostID) == userID {
		newHost := syncplay.OldestMember(filtered)
		newHostID = string(newHost.UserID)
		if _, err = tx.ExecContext(ctx,
			`UPDATE syncplay_rooms SET members = $1, host_id = $2 WHERE id = $3`,
			updated, newHostID, roomID,
		); err != nil {
			return false, "", err
		}
	} else {
		if _, err = tx.ExecContext(ctx,
			`UPDATE syncplay_rooms SET members = $1 WHERE id = $2`,
			updated, roomID,
		); err != nil {
			return false, "", err
		}
	}

	if err = tx.Commit(); err != nil {
		return false, "", err
	}
	return false, newHostID, nil
}

func (r *SyncPlayRoomRepo) UpdateState(ctx context.Context, roomID string, state syncplay.PlaybackState) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.UpdateState")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE syncplay_rooms SET state = $1 WHERE id = $2`,
		stateJSON, roomID,
	)
	return err
}

func (r *SyncPlayRoomRepo) UpdateTrackIndex(ctx context.Context, roomID string, idx int, mediaID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.UpdateTrackIndex")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE syncplay_rooms SET track_index = $1, media_id = $2 WHERE id = $3`,
		idx, mediaID, roomID,
	)
	return err
}

func (r *SyncPlayRoomRepo) AdvanceTrackIndex(ctx context.Context, roomID string, delta int) (int, string, bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.AdvanceTrackIndex")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// One atomic statement: the new index and media_id are derived from the row
	// the UPDATE locks, so two concurrent advances serialise on the row lock and
	// each sees the other's committed increment (+1 then +1 = +2, never +1). The
	// bounds in WHERE make an out-of-range advance a no-op (0 rows → no row
	// returned → changed=false).
	var newIdx int
	var mediaID string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`UPDATE syncplay_rooms
		    SET track_index = track_index + $2,
		        media_id    = queue->>(track_index + $2)
		  WHERE id = $1
		    AND track_index + $2 >= 0
		    AND track_index + $2 < jsonb_array_length(queue)
		  RETURNING track_index, media_id`,
		roomID, delta,
	).Scan(&newIdx, &mediaID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		span.RecordError(err)
		return 0, "", false, err
	}
	return newIdx, mediaID, true, nil
}

func (r *SyncPlayRoomRepo) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM syncplay_rooms WHERE id = $1`, id,
	)
	return err
}

func (r *SyncPlayRoomRepo) DeleteStaleEmptyRooms(ctx context.Context, olderThan time.Time) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.syncplay_room.DeleteStaleEmptyRooms")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// members can be a JSON null (a room created from a nil slice marshals to
	// 'null', not '[]'), and jsonb_array_length() errors on a non-array scalar -
	// so normalise anything that isn't an array to '[]' before measuring length.
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`DELETE FROM syncplay_rooms
		 WHERE created_at < $1
		   AND jsonb_array_length(CASE WHEN jsonb_typeof(members) = 'array' THEN members ELSE '[]'::jsonb END) = 0
		 RETURNING id`,
		olderThan.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

var _ syncplay.RoomRepository = (*SyncPlayRoomRepo)(nil)
