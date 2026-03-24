package shared

import "time"

type ShareLink struct {
	ID          string
	Token       string
	MediaItemID string
	CreatedBy   string
	ExpiresAt   time.Time
	MaxViews    *int
	ViewCount   int
	CreatedAt   time.Time
}

func (s *ShareLink) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *ShareLink) IsViewLimitReached() bool {
	return s.MaxViews != nil && s.ViewCount >= *s.MaxViews
}
