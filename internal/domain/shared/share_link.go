package shared

import (
	"errors"
	"time"
)

type ShareLink struct {
	ID          string
	Token       string
	MediaItemID MediaItemID
	CreatedBy   UserID
	ExpiresAt   time.Time
	MaxViews    *int
	ViewCount   int
	CreatedAt   time.Time
}

// NewShareLink is the canonical constructor. A share link backs a PUBLIC,
// unauthenticated download URL, so its invariants must hold before it is ever
// persisted - an empty token or a non-expiring link would be a security defect.
func NewShareLink(id, token string, mediaItemID MediaItemID, createdBy UserID, expiresAt time.Time, maxViews *int, createdAt time.Time) (*ShareLink, error) {
	s := &ShareLink{
		ID:          id,
		Token:       token,
		MediaItemID: mediaItemID,
		CreatedBy:   createdBy,
		ExpiresAt:   expiresAt,
		MaxViews:    maxViews,
		CreatedAt:   createdAt,
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Validate enforces the share-link invariants. Called by the repository before
// persisting so the guarantee holds regardless of which app-layer caller built
// the struct.
func (s *ShareLink) Validate() error {
	if s.ID == "" {
		return errors.New("share link ID is required")
	}
	if s.Token == "" {
		return errors.New("share link token is required")
	}
	if s.MediaItemID == "" {
		return errors.New("share link media item ID is required")
	}
	if s.CreatedBy == "" {
		return errors.New("share link creator is required")
	}
	if s.ExpiresAt.IsZero() || !s.ExpiresAt.After(s.CreatedAt) {
		return errors.New("share link must expire after its creation time")
	}
	if s.MaxViews != nil && *s.MaxViews < 0 {
		return errors.New("share link max views must not be negative")
	}
	return nil
}

func (s *ShareLink) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *ShareLink) IsViewLimitReached() bool {
	return s.MaxViews != nil && s.ViewCount >= *s.MaxViews
}
