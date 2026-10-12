package user_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/domain/user/fakes"
)

// fakeEpochRepo is an in-memory TokenEpochRepository counting DB lookups so
// tests can assert the cache actually absorbs them.
type fakeEpochRepo struct {
	mu      sync.Mutex
	epochs  map[string]int64
	lookups int
	err     error
}

func newFakeEpochRepo() *fakeEpochRepo {
	return &fakeEpochRepo{epochs: map[string]int64{}}
}

func (f *fakeEpochRepo) GetTokenEpoch(_ context.Context, id shared.UserID) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	if f.err != nil {
		return 0, f.err
	}
	e, ok := f.epochs[string(id)]
	if !ok {
		return 0, shared.ErrNotFound
	}
	return e, nil
}

func (f *fakeEpochRepo) BumpTokenEpoch(_ context.Context, id shared.UserID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.epochs[string(id)]++
	return nil
}

func newGuard(t *testing.T, repo *fakeEpochRepo) *appuser.EpochGuardedTokens {
	t.Helper()
	g, stop := appuser.NewEpochGuardedTokens(fakes.NewFakeTokenGenerator(), repo)
	t.Cleanup(stop)
	return g
}

func TestEpochGuard_MintStampsCurrentEpoch(t *testing.T) {
	repo := newFakeEpochRepo()
	repo.epochs["u1"] = 3
	g := newGuard(t, repo)

	tok, err := g.GenerateAccessToken(user.Claims{UserID: "u1", Username: "u"})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	claims, err := g.ValidateAccessToken(tok)
	if err != nil {
		t.Fatalf("validate freshly minted: %v", err)
	}
	if claims.TokenEpoch != 3 {
		t.Errorf("minted epoch: want 3, got %d", claims.TokenEpoch)
	}
}

func TestEpochGuard_RevokeInvalidatesOutstandingToken(t *testing.T) {
	repo := newFakeEpochRepo()
	repo.epochs["u1"] = 0
	g := newGuard(t, repo)

	tok, err := g.GenerateAccessToken(user.Claims{UserID: "u1"})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if _, vErr := g.ValidateAccessToken(tok); vErr != nil {
		t.Fatalf("validate before revoke: %v", vErr)
	}

	if rErr := g.Revoke(context.Background(), "u1"); rErr != nil {
		t.Fatalf("revoke: %v", rErr)
	}
	// Revoke drops the local cache entry, so rejection is immediate here
	// (other replicas converge within the cache TTL).
	if _, vErr := g.ValidateAccessToken(tok); !errors.Is(vErr, appuser.ErrTokenRevoked) {
		t.Fatalf("want ErrTokenRevoked after bump, got %v", vErr)
	}

	// A re-login mints with the new epoch and is accepted again.
	tok2, err := g.GenerateAccessToken(user.Claims{UserID: "u1"})
	if err != nil {
		t.Fatalf("re-mint: %v", err)
	}
	if _, err := g.ValidateAccessToken(tok2); err != nil {
		t.Fatalf("validate re-minted: %v", err)
	}
}

func TestEpochGuard_CacheAbsorbsLookups(t *testing.T) {
	repo := newFakeEpochRepo()
	repo.epochs["u1"] = 0
	g := newGuard(t, repo)

	tok, _ := g.GenerateAccessToken(user.Claims{UserID: "u1"})
	for range 20 {
		if _, err := g.ValidateAccessToken(tok); err != nil {
			t.Fatalf("validate: %v", err)
		}
	}
	repo.mu.Lock()
	lookups := repo.lookups
	repo.mu.Unlock()
	if lookups != 1 {
		t.Errorf("want exactly 1 DB lookup (mint primes the cache), got %d", lookups)
	}
}

func TestEpochGuard_FailsClosedOnLookupError(t *testing.T) {
	repo := newFakeEpochRepo()
	repo.epochs["u1"] = 0
	g := newGuard(t, repo)

	tok, _ := g.GenerateAccessToken(user.Claims{UserID: "u1"})

	// Simulate the user row disappearing (deleted account): the cached value
	// is gone after Revoke-less Delete, so force a fresh lookup via a new guard.
	g2 := newGuard(t, repo)
	repo.err = errors.New("db down")
	if _, err := g2.ValidateAccessToken(tok); err == nil {
		t.Fatal("want validation failure when the epoch can't be established")
	}
}

func TestEpochGuard_UnknownUserRejected(t *testing.T) {
	repo := newFakeEpochRepo()
	g := newGuard(t, repo)
	// Mint for a user that has no row → fail closed at mint time too.
	if _, err := g.GenerateAccessToken(user.Claims{UserID: "ghost"}); err == nil {
		t.Fatal("want mint failure for unknown user")
	}
}
