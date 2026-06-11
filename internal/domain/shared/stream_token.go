package shared

import "time"

type StreamToken struct {
	Token  string
	UserID UserID // empty for share links
	// ProfileID is the playback profile that minted the token, empty for
	// share links (no profile) and for tokens created before this field
	// existed. The streaming rate limiter keys on it (per-profile budget,
	// matching the back-office per-profile screen/bitrate limits); callers
	// fall back to UserID then IP when it's empty.
	ProfileID   ProfileID
	MediaItemID MediaItemID
	Scope       string // "stream" | "share"
	ExpiresAt   time.Time
	// TTL is the sliding renewal window for this specific token. Stored
	// per-token (not just the store default) so a long-lived cast token
	// doesn't get slid back down to the short default on first validation.
	TTL time.Duration
}
