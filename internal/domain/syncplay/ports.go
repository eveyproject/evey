package syncplay

import (
	"context"
	"time"
)

type RoomRepository interface {
	Create(ctx context.Context, room *Room) error
	Get(ctx context.Context, id string) (*Room, error)
	GetByCode(ctx context.Context, code string) (*Room, error)
	AddMember(ctx context.Context, roomID string, member Member) error
	RemoveMember(ctx context.Context, roomID string, userID string) error
	// RemoveMemberAndDeleteIfEmpty removes the member and, when no members
	// remain, deletes the room - atomically under one row lock so a concurrent
	// join is never clobbered and an emptied room is never left behind.
	// If the departing member was the host and the room is not deleted, the
	// oldest remaining member (earliest JoinedAt; tie-break: ascending UserID)
	// is elected as the new host inside the same lock/transaction so the room
	// is never left with a ghost host.
	// Returns: deleted=true when the room was deleted; newHostID is non-empty
	// when a host election occurred; err for any failure.
	RemoveMemberAndDeleteIfEmpty(ctx context.Context, roomID string, userID string) (deleted bool, newHostID string, err error)
	UpdateState(ctx context.Context, roomID string, state PlaybackState) error
	UpdateTrackIndex(ctx context.Context, roomID string, idx int, mediaID string) error
	// AdvanceTrackIndex atomically applies delta (+1 next / -1 prev) to the
	// room's track index and sets media_id to the new queue entry in one
	// statement, so two concurrent advances (a host with two tabs) can't both
	// read the same index and lose an increment. changed is false (no update)
	// when the result would fall outside the queue bounds.
	AdvanceTrackIndex(ctx context.Context, roomID string, delta int) (newIndex int, mediaID string, changed bool, err error)
	Delete(ctx context.Context, id string) error
	// DeleteStaleEmptyRooms removes rooms with no members that were created
	// before olderThan, and returns the ids it deleted. A room created via
	// REST whose host never opens the ws (or an orphan the delete-if-empty path
	// missed) otherwise lingers forever; the janitor sweeps them. Rooms with a
	// live member are never touched, so olderThan only needs to exceed the
	// create→join window.
	// The IDS, not just a count: each deleted room may have minted guest stream
	// tokens, and a token whose room is gone must go with it.
	DeleteStaleEmptyRooms(ctx context.Context, olderThan time.Time) ([]string, error)
	// CountActiveByHost returns how many rooms this user currently hosts. The
	// Hub uses it to cap per-user room creation: the janitor bounds growth in
	// time, but a scripted create loop could still spike the table between
	// sweeps.
	CountActiveByHost(ctx context.Context, hostID string) (int, error)
}

type MessageSender interface {
	SendMessage(data []byte) error
}
