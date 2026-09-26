package shared

import "time"

// Stream-token scopes.
const (
	// ScopeStream is an authenticated user's normal playback token.
	ScopeStream = "stream"
	// ScopeShare is a public share link.
	ScopeShare = "share"
	// ScopeSyncplayGuest is an unauthenticated "watch together" guest token: it
	// may stream exactly one media item (the room's), but must be refused by the
	// download handler and any endpoint that is not read-only streaming.
	ScopeSyncplayGuest = "syncplay-guest"
	// ScopeBackupDownload lets an admin's browser or phone fetch ONE backup
	// archive by URL (an anchor or a native download cannot send a header).
	// Minted only by the admin backup route, refused everywhere else.
	ScopeBackupDownload = "backup-download"
)

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
	// Cast marks a token minted for a cast receiver (?purpose=cast). It is the
	// discriminator the server-side cast progress tracker keys on: the receiver
	// fetches media on its own with this token, so its fetches are the ONLY
	// playback signal once the sender's browser closes. The Safari/iOS "native"
	// token shares the same long TTL but stays false - that player reports its
	// own progress and must not be double-tracked.
	Cast bool
	// RoomID binds a syncplay-guest token to the room whose code minted it, so
	// closing the room can revoke it. Without it the sharing decision was
	// one-way: the host could end the party but not the access it handed out.
	// Empty on every other scope.
	RoomID    string
	ExpiresAt time.Time
	// TTL is the sliding renewal window for this specific token. Stored
	// per-token (not just the store default) so a long-lived cast token
	// doesn't get slid back down to the short default on first validation.
	TTL time.Duration
}
