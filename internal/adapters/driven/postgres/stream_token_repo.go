package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// StreamTokenRepo persists stream tokens (by hash) so they survive a restart.
type StreamTokenRepo struct {
	db *sql.DB
}

var _ shared.StreamTokenRepository = (*StreamTokenRepo)(nil)

// NewStreamTokenRepo returns a new StreamTokenRepo.
func NewStreamTokenRepo(db *sql.DB) *StreamTokenRepo {
	return &StreamTokenRepo{db: db}
}

func (r *StreamTokenRepo) SaveStreamToken(ctx context.Context, hash string, st shared.StreamToken, issuedAt time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.stream_token.Save")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
INSERT INTO stream_tokens (token_hash, user_id, profile_id, media_item_id, scope, cast_token, room_id, ttl_ms, issued_at, expires_at)
VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (token_hash) DO NOTHING`,
		hash, string(st.UserID), string(st.ProfileID), string(st.MediaItemID), st.Scope, st.Cast, st.RoomID,
		st.TTL.Milliseconds(), issuedAt, st.ExpiresAt)
	return err
}

func (r *StreamTokenRepo) LoadStreamToken(ctx context.Context, hash string) (shared.StreamToken, time.Time, bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.stream_token.Load")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var (
		st       shared.StreamToken
		userID   sql.NullString
		profile  string
		media    string
		ttlMs    int64
		issuedAt time.Time
	)
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
SELECT user_id, profile_id, media_item_id, scope, cast_token, room_id, ttl_ms, issued_at, expires_at
FROM stream_tokens WHERE token_hash = $1`, hash).
		Scan(&userID, &profile, &media, &st.Scope, &st.Cast, &st.RoomID, &ttlMs, &issuedAt, &st.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return shared.StreamToken{}, time.Time{}, false, nil
	}
	if err != nil {
		return shared.StreamToken{}, time.Time{}, false, err
	}
	st.UserID = shared.UserID(userID.String)
	st.ProfileID = shared.ProfileID(profile)
	st.MediaItemID = shared.MediaItemID(media)
	st.TTL = time.Duration(ttlMs) * time.Millisecond
	return st, issuedAt, true, nil
}

func (r *StreamTokenRepo) ExtendStreamToken(ctx context.Context, hash string, expiresAt time.Time) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Never shortens: extensions may land out of order.
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE stream_tokens SET expires_at = GREATEST(expires_at, $2) WHERE token_hash = $1`, hash, expiresAt)
	return err
}

func (r *StreamTokenRepo) DeleteStreamToken(ctx context.Context, hash string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM stream_tokens WHERE token_hash = $1`, hash)
	return err
}

func (r *StreamTokenRepo) DeleteStreamTokensByUser(ctx context.Context, userID shared.UserID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM stream_tokens WHERE user_id = $1`, string(userID))
	return err
}

func (r *StreamTokenRepo) DeleteStreamTokensByRoom(ctx context.Context, roomID string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM stream_tokens WHERE room_id = $1`, roomID)
	return err
}

func (r *StreamTokenRepo) DeleteExpiredStreamTokens(ctx context.Context, now time.Time) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM stream_tokens WHERE expires_at < $1`, now)
	return err
}
