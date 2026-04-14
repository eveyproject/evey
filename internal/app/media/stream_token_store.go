package media

import (
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/shared"
)

const (
	DefaultStreamTokenTTL = 5 * time.Minute
	cleanupInterval       = 1 * time.Minute
)

type StreamTokenStore struct {
	tokens sync.Map // token string → *shared.StreamToken
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

func (s *StreamTokenStore) Create(userID, mediaItemID string) string {
	token := uuid.New().String()
	s.tokens.Store(token, &shared.StreamToken{
		Token:       token,
		UserID:      shared.UserID(userID),
		MediaItemID: shared.MediaItemID(mediaItemID),
		Scope:       "stream",
		ExpiresAt:   time.Now().Add(s.ttl),
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
	st.ExpiresAt = time.Now().Add(s.ttl)
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
