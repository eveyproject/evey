package shared

import (
	"context"
	"time"
)

// StreamTokenRepository keeps minted stream tokens across a server restart.
// The in-memory store used to be the only copy: a restart (every deploy)
// forgot every token, and the native players, whose one credential is the
// token fixed on their source, were refused every segment until they gave up
// (~50 s cut, production 2026-09-23/24). Tokens are keyed by a hash, never
// stored in clear: a copy of the table streams nothing.
type StreamTokenRepository interface {
	SaveStreamToken(ctx context.Context, hash string, st StreamToken, issuedAt time.Time) error
	// LoadStreamToken returns the token (its Token field empty) and its issue time.
	LoadStreamToken(ctx context.Context, hash string) (StreamToken, time.Time, bool, error)
	ExtendStreamToken(ctx context.Context, hash string, expiresAt time.Time) error
	DeleteStreamToken(ctx context.Context, hash string) error
	DeleteStreamTokensByUser(ctx context.Context, userID UserID) error
	DeleteStreamTokensByRoom(ctx context.Context, roomID string) error
	DeleteExpiredStreamTokens(ctx context.Context, now time.Time) error
}
