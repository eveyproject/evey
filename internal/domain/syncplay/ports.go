package syncplay

import "context"

type RoomRepository interface {
	Create(ctx context.Context, room *Room) error
	Get(ctx context.Context, id string) (*Room, error)
	GetByCode(ctx context.Context, code string) (*Room, error)
	AddMember(ctx context.Context, roomID string, member Member) error
	RemoveMember(ctx context.Context, roomID string, userID string) error
	// RemoveMemberAndDeleteIfEmpty removes the member and, when no members
	// remain, deletes the room — atomically under one row lock so a concurrent
	// join is never clobbered and an emptied room is never left behind. Returns
	// whether the room was deleted.
	RemoveMemberAndDeleteIfEmpty(ctx context.Context, roomID string, userID string) (bool, error)
	UpdateState(ctx context.Context, roomID string, state PlaybackState) error
	UpdateTrackIndex(ctx context.Context, roomID string, idx int, mediaID string) error
	// AdvanceTrackIndex atomically applies delta (+1 next / -1 prev) to the
	// room's track index and sets media_id to the new queue entry in one
	// statement, so two concurrent advances (a host with two tabs) can't both
	// read the same index and lose an increment. changed is false (no update)
	// when the result would fall outside the queue bounds.
	AdvanceTrackIndex(ctx context.Context, roomID string, delta int) (newIndex int, mediaID string, changed bool, err error)
	Delete(ctx context.Context, id string) error
}

type MessageSender interface {
	SendMessage(data []byte) error
}
