package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/syncplay"
)

type SyncPlayRoomRepo struct {
	mu    sync.RWMutex
	rooms map[string]*syncplay.Room
}

func NewSyncPlayRoomRepo() *SyncPlayRoomRepo {
	return &SyncPlayRoomRepo{rooms: make(map[string]*syncplay.Room)}
}

func (r *SyncPlayRoomRepo) Create(_ context.Context, room *syncplay.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *room
	cp.Members = make([]syncplay.Member, len(room.Members))
	copy(cp.Members, room.Members)
	r.rooms[room.ID] = &cp
	return nil
}

func (r *SyncPlayRoomRepo) Get(_ context.Context, id string) (*syncplay.Room, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	room, ok := r.rooms[id]
	if !ok {
		return nil, fmt.Errorf("room not found: %s", id)
	}
	cp := *room
	cp.Members = make([]syncplay.Member, len(room.Members))
	copy(cp.Members, room.Members)
	return &cp, nil
}

func (r *SyncPlayRoomRepo) AddMember(_ context.Context, roomID string, member syncplay.Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return fmt.Errorf("room not found: %s", roomID)
	}
	for _, m := range room.Members {
		if m.UserID == member.UserID {
			return nil
		}
	}
	room.Members = append(room.Members, member)
	return nil
}

func (r *SyncPlayRoomRepo) RemoveMember(_ context.Context, roomID string, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return nil
	}
	filtered := room.Members[:0]
	for _, m := range room.Members {
		if m.UserID != userID {
			filtered = append(filtered, m)
		}
	}
	room.Members = filtered
	return nil
}

func (r *SyncPlayRoomRepo) UpdateState(_ context.Context, roomID string, state syncplay.PlaybackState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return fmt.Errorf("room not found: %s", roomID)
	}
	room.State = state
	return nil
}

func (r *SyncPlayRoomRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, id)
	return nil
}
