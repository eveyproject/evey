package fakes

import (
	"context"
	"sync"
	"time"

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

func (r *FakeRoomRepository) CountActiveByHost(_ context.Context, hostID string) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, room := range r.items {
		if string(room.HostID) == hostID {
			n++
		}
	}
	return n, nil
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
		if string(m.UserID) != userID {
			members = append(members, m)
		}
	}
	room.Members = members
	return nil
}

func (r *FakeRoomRepository) RemoveMemberAndDeleteIfEmpty(_ context.Context, roomID string, userID string) (bool, string, error) {
	if r.Err != nil {
		return false, "", r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.items[roomID]
	if !ok {
		return false, "", ErrNotFound
	}
	filtered := make([]syncplay.Member, 0, len(room.Members))
	for _, m := range room.Members {
		if string(m.UserID) != userID {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		delete(r.items, roomID)
		return true, "", nil
	}
	// Elect a new host if the departing member was the host - mirrors the
	// atomicity guarantee provided by the postgres FOR UPDATE transaction.
	newHostID := ""
	if string(room.HostID) == userID {
		newHost := syncplay.OldestMember(filtered)
		newHostID = string(newHost.UserID)
		room.HostID = newHost.UserID
	}
	room.Members = filtered
	return false, newHostID, nil
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

func (r *FakeRoomRepository) UpdateTrackIndex(_ context.Context, roomID string, idx int, mediaID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.items[roomID]
	if !ok {
		return ErrNotFound
	}
	room.TrackIndex = idx
	room.MediaID = shared.MediaItemID(mediaID)
	return nil
}

func (r *FakeRoomRepository) AdvanceTrackIndex(_ context.Context, roomID string, delta int) (int, string, bool, error) {
	if r.Err != nil {
		return 0, "", false, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.items[roomID]
	if !ok {
		return 0, "", false, ErrNotFound
	}
	newIdx := room.TrackIndex + delta
	if newIdx < 0 || newIdx >= len(room.Queue) {
		return 0, "", false, nil
	}
	room.TrackIndex = newIdx
	room.MediaID = room.Queue[newIdx]
	return newIdx, string(room.MediaID), true, nil
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

func (r *FakeRoomRepository) DeleteStaleEmptyRooms(_ context.Context, olderThan time.Time) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for id, room := range r.items {
		if len(room.Members) == 0 && room.CreatedAt.Before(olderThan) {
			delete(r.items, id)
			ids = append(ids, id)
		}
	}
	return ids, nil
}
