package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakePlaySessionRepository struct {
	mu    sync.RWMutex
	items map[string]*playback.PlaySession
	// commands holds one pending remote command per session, mirroring the
	// real repo's set-then-destructive-consume contract.
	commands map[string]string
	Err      error
	// LockCalls counts AcquireUserStreamLock invocations so tests can assert the
	// count-then-create path serializes - e.g. that the device-cap branch takes
	// the advisory lock too, not only the stream-cap branch.
	LockCalls int
}

func NewFakePlaySessionRepository() *FakePlaySessionRepository {
	return &FakePlaySessionRepository{items: make(map[string]*playback.PlaySession), commands: make(map[string]string)}
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

func (r *FakePlaySessionRepository) UpdateProgress(_ context.Context, sessionID string, positionMs int64, isPaused bool, castDeviceName string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	s, ok := r.items[sessionID]
	if !ok || s.EndedAt != nil {
		// Guard mirrors the SQL `WHERE id=$ AND ended_at IS NULL`: a tick on an
		// ended (or absent) session is a no-op and cannot touch ended_at.
		return false, nil
	}
	if pos, err := shared.NewDurationMs(positionMs); err == nil {
		s.Position = &pos
	}
	s.IsPaused = isPaused
	s.CastDeviceName = castDeviceName
	return true, nil
}

func (r *FakePlaySessionRepository) ReopenEndedBefore(_ context.Context, sessionID string, before time.Time, positionMs int64, isPaused bool, castDeviceName string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	s, ok := r.items[sessionID]
	if !ok || s.EndedAt == nil || !s.EndedAt.Before(before) {
		return false, nil
	}
	s.EndedAt = nil
	if pos, err := shared.NewDurationMs(positionMs); err == nil {
		s.Position = &pos
	}
	s.IsPaused = isPaused
	s.CastDeviceName = castDeviceName
	return true, nil
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
				SessionID:      s.ID,
				ProfileID:      s.ProfileID,
				ProfileName:    "profile-" + string(s.ProfileID),
				MediaItemID:    s.MediaItemID,
				MediaTitle:     "media-" + string(s.MediaItemID),
				MediaType:      "movie",
				StartedAt:      s.StartedAt,
				DeviceName:     s.DeviceName,
				DeviceType:     s.DeviceType,
				CastDeviceName: s.CastDeviceName,
				VideoEngine:    s.Serving.VideoEngine,
				VideoBackend:   s.Serving.VideoBackend,
				AudioEngine:    s.Serving.AudioEngine,
				ClientIP:       s.ClientIP,
				PlayMethod:     s.PlayMethod,
				VideoCodec:     s.VideoCodec,
				AudioCodec:     s.AudioCodec,
				Resolution:     s.Resolution,
				BandwidthBps:   s.BandwidthBps,
			}
			if s.Position != nil {
				info.Position = *s.Position
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

func (r *FakePlaySessionRepository) CloseAllActive(_ context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return 0, r.Err
	}
	closed := 0
	for _, s := range r.items {
		if s.EndedAt == nil {
			now := time.Now()
			s.EndedAt = &now
			closed++
		}
	}
	return closed, nil
}

func (r *FakePlaySessionRepository) PurgeEndedBefore(_ context.Context, cutoff time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return 0, r.Err
	}
	purged := 0
	for id, s := range r.items {
		if s.EndedAt != nil && s.EndedAt.Before(cutoff) {
			delete(r.items, id)
			purged++
		}
	}
	return purged, nil
}

func (r *FakePlaySessionRepository) SetSessionAudioDecision(_ context.Context, sessionID, decision string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	s, ok := r.items[sessionID]
	if !ok || s.EndedAt != nil {
		return false, nil
	}
	s.AudioDecision = decision
	return true, nil
}

func (r *FakePlaySessionRepository) SetSessionServing(_ context.Context, sessionID string, p playback.ServingPath) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	s, ok := r.items[sessionID]
	if !ok || s.EndedAt != nil {
		return false, nil
	}
	s.VideoDecision, s.AudioDecision, s.Serving = p.VideoDecision, p.AudioDecision, p
	if p.AudioCodec != "" { // same contract as the SQL: empty leaves the label alone
		s.AudioCodec = p.AudioCodec
	}
	return true, nil
}

func (r *FakePlaySessionRepository) SetSessionAudioPath(_ context.Context, sessionID, decision, engine, audioCodec string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	s, ok := r.items[sessionID]
	if !ok || s.EndedAt != nil {
		return false, nil
	}
	s.AudioDecision, s.Serving.AudioDecision, s.Serving.AudioEngine = decision, decision, engine
	if audioCodec != "" {
		s.AudioCodec = audioCodec
	}
	return true, nil
}

func (r *FakePlaySessionRepository) SetRemoteCommand(_ context.Context, sessionID, command string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err == nil {
		r.commands[sessionID] = command
	}
	return r.Err
}

func (r *FakePlaySessionRepository) ConsumeRemoteCommand(_ context.Context, sessionID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return "", r.Err
	}
	cmd := r.commands[sessionID]
	delete(r.commands, sessionID)
	return cmd, nil
}

// AcquireUserStreamLock is a no-op lock in the fake (the items mutex already
// serializes operations); it only records the call so tests can assert the
// command service reaches for the advisory lock on the expected paths.
func (r *FakePlaySessionRepository) AcquireUserStreamLock(_ context.Context, _ string) error {
	r.mu.Lock()
	r.LockCalls++
	r.mu.Unlock()
	return r.Err
}

// CountActiveByUser counts every open session. The fake cannot filter by
// user because PlaySession only carries a ProfileID and the profile-to-user
// mapping lives in a separate repo. The real postgres implementation joins
// on profiles.user_id - tests exercising multi-user quota isolation must
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
