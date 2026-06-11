package media

import (
	"log/slog"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/google/uuid"
)

const (
	DefaultStreamTokenTTL = 5 * time.Minute
	cleanupInterval       = 1 * time.Minute
	// CastStreamTokenTTL is the renewal window for tokens handed to a
	// Chromecast. The device fetches HLS segments on its own and buffers
	// minutes ahead, then goes quiet — with the 5-minute default it would
	// expire mid-film during a buffer-quiet stretch (the local player
	// dodges this by re-validating on every xhr, the cast can't). 12h
	// comfortably covers any single playback session.
	CastStreamTokenTTL = 12 * time.Hour
)

type StreamTokenStore struct {
	// tokens maps token string → *shared.StreamToken. CreateWithTTL is the ONLY
	// insert site (always a string key and a *shared.StreamToken value), so the
	// unchecked type assertions in Validate / cleanupLoop are invariant-safe by
	// construction — a bare v.(*shared.StreamToken) / key.(string) cannot panic.
	tokens sync.Map
	ttl    time.Duration
	done   chan struct{}
}

func NewStreamTokenStore(ttl time.Duration) *StreamTokenStore {
	if ttl <= 0 {
		ttl = DefaultStreamTokenTTL
	}
	s := &StreamTokenStore{
		ttl:  ttl,
		done: make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *StreamTokenStore) Create(userID, profileID, mediaItemID string) string {
	return s.CreateWithTTL(userID, profileID, mediaItemID, s.ttl)
}

// CreateWithTTL mints a token with an explicit sliding TTL. Used for cast
// playback, where the short default would expire during the Chromecast's
// buffer-quiet stretches. profileID may be empty (the streaming rate limiter
// then falls back to userID).
func (s *StreamTokenStore) CreateWithTTL(userID, profileID, mediaItemID string, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = s.ttl
	}
	token := uuid.New().String()
	s.tokens.Store(token, &shared.StreamToken{
		Token:       token,
		UserID:      shared.UserID(userID),
		ProfileID:   shared.ProfileID(profileID),
		MediaItemID: shared.MediaItemID(mediaItemID),
		Scope:       "stream",
		ExpiresAt:   time.Now().Add(ttl),
		TTL:         ttl,
	})
	return token
}

// Validate checks the token, verifies it matches the mediaItemID, and renews the TTL.
// If mediaItemID is empty, any media is accepted (used by middleware).
func (s *StreamTokenStore) Validate(token, mediaItemID string) (*shared.StreamToken, bool) {
	v, ok := s.tokens.Load(token)
	if !ok {
		return nil, false
	}
	st := v.(*shared.StreamToken)
	if time.Now().After(st.ExpiresAt) {
		s.tokens.Delete(token)
		return nil, false
	}
	if mediaItemID != "" && st.MediaItemID != shared.MediaItemID(mediaItemID) {
		return nil, false
	}
	// Slide by the token's own TTL, not the store default — otherwise a
	// long-lived cast token would be downgraded to the 5-minute window
	// on its first segment fetch.
	ttl := st.TTL
	if ttl <= 0 {
		ttl = s.ttl
	}
	st.ExpiresAt = time.Now().Add(ttl)
	return st, true
}

// ValidateAny checks the token without media scope (middleware use).
func (s *StreamTokenStore) ValidateAny(token string) (*shared.StreamToken, bool) {
	return s.Validate(token, "")
}

func (s *StreamTokenStore) Revoke(token string) {
	s.tokens.Delete(token)
}

func (s *StreamTokenStore) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func (s *StreamTokenStore) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			now := time.Now()
			s.tokens.Range(func(key, value any) bool {
				st := value.(*shared.StreamToken)
				if now.After(st.ExpiresAt) {
					s.tokens.Delete(key)
					slog.Debug("stream token expired", slog.String("token", key.(string)))
				}
				return true
			})
		}
	}
}
