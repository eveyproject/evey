package shared

import "time"

type StreamToken struct {
	Token       string
	UserID      UserID // empty for share links
	MediaItemID MediaItemID
	Scope       string // "stream" | "share"
	ExpiresAt   time.Time
}
