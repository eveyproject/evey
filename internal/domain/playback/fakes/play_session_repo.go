package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/playback"
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
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[session.ID] = session
	return nil
}

func (r *FakePlaySessionRepository) Update(_ context.Context, session *playback.PlaySession) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[session.ID]; !ok {
		return ErrNotFound
	}
	r.items[session.ID] = session
	return nil
}

func (r *FakePlaySessionRepository) GetByID(_ context.Context, id string) (*playback.PlaySession, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

func (r *FakePlaySessionRepository) ListActiveSessions(_ context.Context) ([]*playback.ActiveSessionInfo, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playback.ActiveSessionInfo
	for _, s := range r.items {
		if s.EndedAt == nil {
			info := &playback.ActiveSessionInfo{
				SessionID:    s.ID,
				ProfileID:    s.ProfileID,
				ProfileName:  "profile-" + s.ProfileID,
				MediaItemID:  s.MediaItemID,
				MediaTitle:   "media-" + s.MediaItemID,
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
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playback.PlaySession
	for _, s := range r.items {
		if s.ProfileID == profileID && s.EndedAt == nil {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *FakePlaySessionRepository) GetHistory(_ context.Context, filter playback.PlaySessionFilter) ([]*playback.PlaySession, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playback.PlaySession
	for _, s := range r.items {
		if filter.ProfileID != nil && s.ProfileID != *filter.ProfileID {
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
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (r *FakePlaySessionRepository) CountActiveByUser(_ context.Context, userID string) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, s := range r.items {
		if s.EndedAt == nil {
			count++
		}
	}
	return count, nil
}
