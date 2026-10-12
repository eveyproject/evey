package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/cache"
)

// ErrTokenRevoked is returned by EpochGuardedTokens.ValidateAccessToken when a
// structurally valid JWT carries a stale token_epoch - i.e. the user's access
// has been revoked (disable, role change, password change, admin kill) since
// the token was minted.
var ErrTokenRevoked = errors.New("access token revoked")

// ErrEpochUnavailable wraps a failure to READ the epoch, as opposed to a token
// that is genuinely revoked. The distinction is the whole point: a revoked
// token is one user's expected 401, while an unavailable epoch means the
// database is not answering and EVERY authenticated request in the server is
// about to fail with the same generic message. Callers must be able to tell
// "this credential is stale" from "this server is broken" - they read the same
// on the wire otherwise, and the operator goes looking for a JWT key problem
// that does not exist.
var ErrEpochUnavailable = errors.New("token epoch unavailable")

const (
	epochCacheTTL     = 30 * time.Second
	epochCacheMaxSize = 4096
	epochLookupBudget = 3 * time.Second
)

// EpochGuardedTokens decorates a TokenGenerator with users.token_epoch
// enforcement, closing the "valid until expiry" window of stateless JWTs:
//
//   - GenerateAccessToken stamps the user's CURRENT epoch into the claims, so
//     every mint site (login, refresh, OIDC, QuickConnect, invites…) is covered
//     without touching it.
//   - ValidateAccessToken rejects tokens whose epoch no longer matches,
//     bounding the revocation window to the cache TTL (~30s) instead of the
//     access-token expiry (15min) - at the cost of one indexed single-row read
//     per user per TTL.
//   - Revoke bumps the counter and drops the local cache entry, so revocation
//     is immediate on this instance and ≤TTL on any other replica.
//
// The TokenGenerator interface has no context (pure-crypto contract), so DB
// lookups run under a short background budget. Lookups fail closed: if the
// epoch can't be established the token is not accepted.
type EpochGuardedTokens struct {
	user.TokenGenerator
	epochs user.TokenEpochRepository
	cache  *cache.TTLCache[int64]
}

// NewEpochGuardedTokens wraps inner. The returned stop func tears down the
// cache janitor on shutdown.
func NewEpochGuardedTokens(inner user.TokenGenerator, epochs user.TokenEpochRepository) (*EpochGuardedTokens, func()) {
	c, stop := cache.NewWithMaxSize[int64](time.Minute, epochCacheMaxSize)
	return &EpochGuardedTokens{
		TokenGenerator: inner,
		epochs:         epochs,
		cache:          c,
	}, stop
}

func (g *EpochGuardedTokens) GenerateAccessToken(claims user.Claims) (string, error) {
	epoch, err := g.currentEpoch(claims.UserID)
	if err != nil {
		return "", fmt.Errorf("token epoch lookup: %w", err)
	}
	claims.TokenEpoch = epoch
	return g.TokenGenerator.GenerateAccessToken(claims)
}

func (g *EpochGuardedTokens) ValidateAccessToken(token string) (*user.Claims, error) {
	claims, err := g.TokenGenerator.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}
	epoch, err := g.currentEpoch(claims.UserID)
	if err != nil {
		// Fail closed: a token whose epoch can't be established (user deleted,
		// DB unreachable beyond the cache) is not accepted. Tagged so the
		// middleware can log the real cause instead of reporting yet another
		// "invalid token".
		return nil, fmt.Errorf("%w: %w", ErrEpochUnavailable, err)
	}
	if claims.TokenEpoch != epoch {
		return nil, ErrTokenRevoked
	}
	return claims, nil
}

// Revoke invalidates every outstanding access token of the user: it bumps the
// persistent counter and drops the local cache entry so the very next request
// on this instance re-reads the new epoch.
func (g *EpochGuardedTokens) Revoke(ctx context.Context, userID string) error {
	if err := g.epochs.BumpTokenEpoch(ctx, shared.UserID(userID)); err != nil {
		return fmt.Errorf("bump token epoch: %w", err)
	}
	g.cache.Delete(userID)
	return nil
}

func (g *EpochGuardedTokens) currentEpoch(userID string) (int64, error) {
	if epoch, ok := g.cache.Get(userID); ok {
		return epoch, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), epochLookupBudget)
	defer cancel()
	epoch, err := g.epochs.GetTokenEpoch(ctx, shared.UserID(userID))
	if err != nil {
		return 0, err
	}
	g.cache.Set(userID, epoch, epochCacheTTL)
	return epoch, nil
}

// TokenRevoker is the slice of EpochGuardedTokens the user services need to
// invalidate outstanding access tokens on security events. Optional everywhere
// it is injected: a nil revoker preserves the historical "valid until expiry"
// behaviour.
type TokenRevoker interface {
	Revoke(ctx context.Context, userID string) error
}
