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
	Delete(ctx context.Context, id string) error
}

type MessageSender interface {
	SendMessage(data []byte) error
}
