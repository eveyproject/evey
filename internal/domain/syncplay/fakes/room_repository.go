package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/syncplay"
)

var ErrNotFound = shared.ErrNotFound

type FakeRoomRepository struct {
	mu    sync.RWMutex
	items map[string]*syncplay.Room
	Err   error
}

func NewFakeRoomRepository() *FakeRoomRepository {
	return &FakeRoomRepository{items: make(map[string]*syncplay.Room)}
}

func (r *FakeRoomRepository) Create(_ context.Context, room *syncplay.Room) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[room.ID] = room
	return nil
}

func (r *FakeRoomRepository) Get(_ context.Context, id string) (*syncplay.Room, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	room, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return room, nil
}

func (r *FakeRoomRepository) GetByCode(_ context.Context, code string) (*syncplay.Room, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, room := range r.items {
		if room.Code == code {
			return room, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeRoomRepository) AddMember(_ context.Context, roomID string, member syncplay.Member) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.items[roomID]
	if !ok {
		return ErrNotFound
	}
	room.Members = append(room.Members, member)
	return nil
}

func (r *FakeRoomRepository) RemoveMember(_ context.Context, roomID string, userID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.items[roomID]
	if !ok {
		return ErrNotFound
	}
	members := make([]syncplay.Member, 0, len(room.Members))
	for _, m := range room.Members {
		if m.UserID != userID {
			members = append(members, m)
		}
	}
	room.Members = members
	return nil
}

func (r *FakeRoomRepository) UpdateState(_ context.Context, roomID string, state syncplay.PlaybackState) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.items[roomID]
	if !ok {
		return ErrNotFound
	}
	room.State = state
	return nil
}

func (r *FakeRoomRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
