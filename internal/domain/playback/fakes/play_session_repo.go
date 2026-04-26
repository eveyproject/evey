package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/playback"
)

type FakePlaySessionRepository struct {
	mu    sync.RWMutex
	items map[string]*playback.PlaySession
	Err   error
}

func NewFakePlaySessionRepository() *FakePlaySessionRepository {
	return &FakePlaySessionRepository{items: make(map[string]*playback.PlaySession)}
}

func (r *FakePlaySessionRepository) Create(_ context.Context, session *playback.PlaySession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[session.ID] = session
	return nil
}

func (r *FakePlaySessionRepository) Update(_ context.Context, session *playback.PlaySession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[session.ID]; !ok {
		return ErrNotFound
	}
	r.items[session.ID] = session
	return nil
}

func (r *FakePlaySessionRepository) GetByID(_ context.Context, id string) (*playback.PlaySession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	s, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	// Return a copy: callers (incl. polling tests) inspect fields like
	// EndedAt while CloseStale concurrently mutates the stored session.
	// Sharing the pointer triggers a -race warning even though callers
	// don't write through it.
	cp := *s
	return &cp, nil
}

func (r *FakePlaySessionRepository) ListActiveSessions(_ context.Context) ([]*playback.ActiveSessionInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*playback.ActiveSessionInfo
	for _, s := range r.items {
		if s.EndedAt == nil {
			info := &playback.ActiveSessionInfo{
				SessionID:    s.ID,
				ProfileID:    s.ProfileID,
				ProfileName:  "profile-" + string(s.ProfileID),
				MediaItemID:  s.MediaItemID,
				MediaTitle:   "media-" + string(s.MediaItemID),
				MediaType:    "movie",
				StartedAt:    s.StartedAt,
				DeviceName:   s.DeviceName,
				DeviceType:   s.DeviceType,
				ClientIP:     s.ClientIP,
				PlayMethod:   s.PlayMethod,
				VideoCodec:   s.VideoCodec,
				AudioCodec:   s.AudioCodec,
				Resolution:   s.Resolution,
				BandwidthBps: s.BandwidthBps,
			}
			if s.PositionMs != nil {
				info.PositionMs = *s.PositionMs
			}
			if s.PlayMethod == string(playback.Transcode) {
				t := s.VideoCodec + " -> h264"
				info.TranscodeInfo = &t
			}
			result = append(result, info)
		}
	}
	return result, nil
}

func (r *FakePlaySessionRepository) GetActiveByProfile(_ context.Context, profileID string) ([]*playback.PlaySession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*playback.PlaySession
	for _, s := range r.items {
		if string(s.ProfileID) == profileID && s.EndedAt == nil {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *FakePlaySessionRepository) GetHistory(_ context.Context, filter playback.PlaySessionFilter) ([]*playback.PlaySession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*playback.PlaySession
	for _, s := range r.items {
		if filter.ProfileID != nil && string(s.ProfileID) != *filter.ProfileID {
			continue
		}
		if filter.StartFrom != nil && s.StartedAt.Before(*filter.StartFrom) {
			continue
		}
		if filter.StartTo != nil && s.StartedAt.After(*filter.StartTo) {
			continue
		}
		result = append(result, s)
	}
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return nil, nil
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *FakePlaySessionRepository) CloseStale(_ context.Context, olderThan time.Duration) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return 0, r.Err
	}
	cutoff := time.Now().Add(-olderThan)
	closed := 0
	for _, s := range r.items {
		if s.EndedAt == nil && s.StartedAt.Before(cutoff) {
			now := time.Now()
			s.EndedAt = &now
			closed++
		}
	}
	return closed, nil
}

func (r *FakePlaySessionRepository) SetRemoteCommand(_ context.Context, sessionID, command string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.items[sessionID]; ok {
		_ = s
		_ = command
	}
	return r.Err
}

func (r *FakePlaySessionRepository) ConsumeRemoteCommand(_ context.Context, _ string) (string, error) {
	return "", r.Err
}

// AcquireUserStreamLock is a no-op in the fake; the test mutex in items
// already serializes operations for the FakePlaySessionRepository.
func (r *FakePlaySessionRepository) AcquireUserStreamLock(_ context.Context, _ string) error {
	return r.Err
}

// CountActiveByUser counts every open session. The fake cannot filter by
// user because PlaySession only carries a ProfileID and the profile-to-user
// mapping lives in a separate repo. The real postgres implementation joins
// on profiles.user_id — tests exercising multi-user quota isolation must
// wire their own wrapper or use integration tests. Matches single-user
// tests correctly (profileIDs all belong to the same user).
func (r *FakePlaySessionRepository) CountActiveByUser(_ context.Context, _ string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return 0, r.Err
	}
	count := 0
	for _, s := range r.items {
		if s.EndedAt == nil {
			count++
		}
	}
	return count, nil
}
