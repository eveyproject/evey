package shared

import "time"

type StreamToken struct {
	Token       string
	UserID      UserID // empty for share links
	MediaItemID MediaItemID
	Scope       string // "stream" | "share"
	ExpiresAt   time.Time
	// TTL is the sliding renewal window for this specific token. Stored
	// per-token (not just the store default) so a long-lived cast token
	// doesn't get slid back down to the short default on first validation.
	TTL time.Duration
}
