package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/syncplay"
)

type SyncPlayRoomRepo struct {
	db *sql.DB
}

func NewSyncPlayRoomRepo(db *sql.DB) *SyncPlayRoomRepo {
	return &SyncPlayRoomRepo{db: db}
}

func (r *SyncPlayRoomRepo) Create(ctx context.Context, room *syncplay.Room) error {
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
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO syncplay_rooms (id, code, name, host_id, media_id, members, state, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		room.ID, room.Code, room.Name, room.HostID, room.MediaID, membersJSON, stateJSON, room.CreatedAt,
	)
	return err
}

func (r *SyncPlayRoomRepo) Get(ctx context.Context, id string) (*syncplay.Room, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanRoom(ctx, `SELECT id, code, name, host_id, media_id, members, state, created_at
		 FROM syncplay_rooms WHERE id = $1`, id)
}

func (r *SyncPlayRoomRepo) GetByCode(ctx context.Context, code string) (*syncplay.Room, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanRoom(ctx, `SELECT id, code, name, host_id, media_id, members, state, created_at
		 FROM syncplay_rooms WHERE code = $1`, code)
}

func (r *SyncPlayRoomRepo) scanRoom(ctx context.Context, query string, arg string) (*syncplay.Room, error) {
	var room syncplay.Room
	var membersJSON, stateJSON []byte
	err := r.db.QueryRowContext(ctx, query, arg).Scan(
		&room.ID, &room.Code, &room.Name, &room.HostID, &room.MediaID,
		&membersJSON, &stateJSON, &room.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
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
	return &room, nil
}

func (r *SyncPlayRoomRepo) AddMember(ctx context.Context, roomID string, member syncplay.Member) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	room, err := r.Get(ctx, roomID)
	if err != nil {
		return err
	}
	if room == nil {
		return nil
	}
	for _, m := range room.Members {
		if m.UserID == member.UserID {
			return nil
		}
	}
	room.Members = append(room.Members, member)
	membersJSON, err := json.Marshal(room.Members)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE syncplay_rooms SET members = $1 WHERE id = $2`,
		membersJSON, roomID,
	)
	return err
}

func (r *SyncPlayRoomRepo) RemoveMember(ctx context.Context, roomID string, userID string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	room, err := r.Get(ctx, roomID)
	if err != nil {
		return err
	}
	if room == nil {
		return nil
	}
	filtered := room.Members[:0]
	for _, m := range room.Members {
		if m.UserID != userID {
			filtered = append(filtered, m)
		}
	}
	membersJSON, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE syncplay_rooms SET members = $1 WHERE id = $2`,
		membersJSON, roomID,
	)
	return err
}

func (r *SyncPlayRoomRepo) UpdateState(ctx context.Context, roomID string, state syncplay.PlaybackState) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE syncplay_rooms SET state = $1 WHERE id = $2`,
		stateJSON, roomID,
	)
	return err
}

func (r *SyncPlayRoomRepo) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM syncplay_rooms WHERE id = $1`, id,
	)
	return err
}

var _ syncplay.RoomRepository = (*SyncPlayRoomRepo)(nil)
