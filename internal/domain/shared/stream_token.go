package shared

import "time"

type StreamToken struct {
	Token       string
	UserID      string // empty for share links
	MediaItemID string
	Scope       string // "stream" | "share"
	ExpiresAt   time.Time
}
