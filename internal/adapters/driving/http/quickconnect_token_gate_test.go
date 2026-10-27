package httpapi

// Tests for qcCheckerAdapter.checkAndGenerateTokens - the QuickConnect device-
// pairing token gate. The security-critical invariant is that a non-Authorized
// or UserID-nil record MUST NOT cause GenerateTokensForUser to be called.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/config"
	domainshared "github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingUserRepo wraps FakeUserRepository and counts GetByID calls.
// GenerateTokensForUser always calls GetByID first, so a zero count proves
// the token-generation path was never entered.
type countingUserRepo struct {
	*ufakes.FakeUserRepository
	getByIDCalls atomic.Int64
}

func (r *countingUserRepo) GetByID(ctx context.Context, id domainshared.UserID) (*domainuser.User, error) {
	r.getByIDCalls.Add(1)
	return r.FakeUserRepository.GetByID(ctx, id)
}

// qcGateFixture holds everything needed to exercise checkAndGenerateTokens.
type qcGateFixture struct {
	qcRepo      *ufakes.FakeQuickConnectRepository
	userRepo    *countingUserRepo
	refreshTkns *ufakes.FakeRefreshTokenRepository
	adapter     *qcCheckerAdapter
	longTTL     time.Duration
	defaultTTL  time.Duration
}

const (
	qcTestDefaultTTL = 7 * 24 * time.Hour
	qcTestLongTTL    = 180 * 24 * time.Hour
)

func newQCGateFixture(t *testing.T) *qcGateFixture {
	t.Helper()

	qcRepo := ufakes.NewFakeQuickConnectRepository()
	baseUserRepo := ufakes.NewFakeUserRepository()
	userRepo := &countingUserRepo{FakeUserRepository: baseUserRepo}
	profiles := ufakes.NewFakeProfileRepository()
	refreshTkns := ufakes.NewFakeRefreshTokenRepository()
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := sharedfakes.NewFakeEventBus()

	querySvc := appuser.NewQueryService(
		userRepo,
		profiles,
		ufakes.NewFakeAPIKeyRepository(),
		ufakes.NewFakeInviteLinkRepository(),
		qcRepo,
	)

	cmdSvc := appuser.NewCommandService(
		userRepo,
		profiles,
		refreshTkns,
		roles,
		userRoles,
		hasher,
		tokens,
		events,
		qcTestDefaultTTL,
		qcRepo,
	)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			RefreshTokenLongTTL: qcTestLongTTL,
		},
	}

	return &qcGateFixture{
		qcRepo:      qcRepo,
		userRepo:    userRepo,
		refreshTkns: refreshTkns,
		adapter: &qcCheckerAdapter{
			query: querySvc,
			auth:  cmdSvc,
			cfg:   cfg,
		},
		longTTL:    qcTestLongTTL,
		defaultTTL: qcTestDefaultTTL,
	}
}

func seedQCRecord(t *testing.T, repo *ufakes.FakeQuickConnectRepository, code string, status domainuser.QuickConnectStatus, userID *domainshared.UserID) {
	t.Helper()
	req := &domainuser.QuickConnectRequest{
		ID:        "qcid-" + code,
		Code:      code,
		Status:    status,
		UserID:    userID,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	require.NoError(t, repo.Create(context.Background(), req))
}

func seedQCUser(t *testing.T, repo *countingUserRepo, userID string) {
	t.Helper()
	u := &domainuser.User{
		ID:       domainshared.UserID(userID),
		Username: userID,
		Enabled:  true,
	}
	require.NoError(t, repo.Create(context.Background(), u))
}

// refreshTokenExpiresAt looks up the refresh token stored in the fake repo by
// hashing the raw token value the same way CommandService does.
func refreshTokenExpiresAt(t *testing.T, repo *ufakes.FakeRefreshTokenRepository, rawToken string) time.Time {
	t.Helper()
	h := sha256.Sum256([]byte(rawToken))
	rt, err := repo.GetByTokenHash(context.Background(), hex.EncodeToString(h[:]))
	require.NoError(t, err, "refresh token not found in fake repo")
	return rt.ExpiresAt
}

// --- Tests ---

func TestQCGate_CheckQuickConnectError_ReturnsError(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	f.qcRepo.Err = errors.New("db connection lost")

	access, refresh, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "any-code", "", false)

	require.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, access)
	assert.Empty(t, refresh)
	// Security gate: GenerateTokensForUser must not be called when CheckQuickConnect errors.
	assert.Equal(t, int64(0), f.userRepo.getByIDCalls.Load(),
		"GenerateTokensForUser must not be called when CheckQuickConnect errors")
}

func TestQCGate_StatusPending_NoTokensMinted(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	uid := domainshared.UserID("usr-alice")
	seedQCRecord(t, f.qcRepo, "code-pending", domainuser.Pending, &uid)

	access, refresh, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-pending", "", false)

	require.NoError(t, err)
	assert.False(t, ok, "Pending status must not pass the gate")
	assert.Empty(t, access)
	assert.Empty(t, refresh)
	// Security-critical: a Pending code must never cause token minting.
	assert.Equal(t, int64(0), f.userRepo.getByIDCalls.Load(),
		"GenerateTokensForUser must not be called for a non-Authorized QuickConnect code")
}

func TestQCGate_StatusExpired_NoTokensMinted(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	uid := domainshared.UserID("usr-bob")
	seedQCRecord(t, f.qcRepo, "code-expired", domainuser.Expired, &uid)

	access, refresh, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-expired", "", false)

	require.NoError(t, err)
	assert.False(t, ok, "Expired status must not pass the gate")
	assert.Empty(t, access)
	assert.Empty(t, refresh)
	assert.Equal(t, int64(0), f.userRepo.getByIDCalls.Load(),
		"GenerateTokensForUser must not be called for a non-Authorized QuickConnect code")
}

func TestQCGate_AuthorizedButUserIDNil_NoTokensMinted(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	// Authorized status but UserID is nil (record not yet bound to a user).
	seedQCRecord(t, f.qcRepo, "code-authorized-noid", domainuser.Authorized, nil)

	access, refresh, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-authorized-noid", "", false)

	require.NoError(t, err)
	assert.False(t, ok, "Authorized+nil UserID must not pass the gate")
	assert.Empty(t, access)
	assert.Empty(t, refresh)
	assert.Equal(t, int64(0), f.userRepo.getByIDCalls.Load(),
		"GenerateTokensForUser must not be called when UserID is nil")
}

func TestQCGate_Authorized_NonPersistent_MintsTokens(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	uid := domainshared.UserID("usr-carol")
	seedQCRecord(t, f.qcRepo, "code-ok", domainuser.Authorized, &uid)
	seedQCUser(t, f.userRepo, string(uid))

	before := time.Now()
	access, refresh, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-ok", "", false)
	after := time.Now()

	require.NoError(t, err)
	assert.True(t, ok)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)

	// GenerateTokensForUser was called - user repo was accessed.
	assert.Positive(t, f.userRepo.getByIDCalls.Load())

	// Exactly one refresh token stored.
	assert.Equal(t, 1, f.refreshTkns.Count())

	// TTL must be the default (7d), not the long one (180d).
	// The two are far enough apart (~173 days) that a simple boundary check is reliable.
	expiry := refreshTokenExpiresAt(t, f.refreshTkns, refresh)
	assert.True(t, expiry.After(before.Add(f.defaultTTL-time.Minute)),
		"expiry should be at least defaultTTL from before")
	assert.True(t, expiry.Before(after.Add(f.defaultTTL+time.Minute)),
		"expiry should be at most defaultTTL from after")
	// Confirm the long TTL was NOT used (expiry << longTTL boundary).
	assert.True(t, expiry.Before(before.Add(f.longTTL)),
		"persistent=false must not use the long TTL")
}

func TestQCGate_Authorized_Persistent_UsesLongTTL(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	uid := domainshared.UserID("usr-dave")
	seedQCRecord(t, f.qcRepo, "code-ok-long", domainuser.Authorized, &uid)
	seedQCUser(t, f.userRepo, string(uid))

	before := time.Now()
	access, refresh, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-ok-long", "", true)
	after := time.Now()

	require.NoError(t, err)
	assert.True(t, ok)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)
	assert.Equal(t, 1, f.refreshTkns.Count())

	// TTL must be the long one (180d), not the default (7d).
	expiry := refreshTokenExpiresAt(t, f.refreshTkns, refresh)
	assert.True(t, expiry.After(before.Add(f.longTTL-time.Minute)),
		"persistent=true must use longTTL: expiry should be at least longTTL from before")
	assert.True(t, expiry.Before(after.Add(f.longTTL+time.Minute)),
		"expiry should be at most longTTL from after")
}

func TestQCGate_GenerateTokensError_Propagated(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	uid := domainshared.UserID("usr-eve")
	seedQCRecord(t, f.qcRepo, "code-fail", domainuser.Authorized, &uid)
	// Seed the user as disabled - CommandService.GenerateTokensForUser returns
	// ErrUserDisabled for disabled users, which is then propagated.
	u := &domainuser.User{
		ID:       uid,
		Username: string(uid),
		Enabled:  false,
	}
	require.NoError(t, f.userRepo.Create(context.Background(), u))

	access, refresh, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-fail", "", false)

	require.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, access)
	assert.Empty(t, refresh)
}

// Two collectors racing on one authorized code: exactly one gets a session.
func TestQCGate_ConcurrentChecksMintOnce(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)
	uid := domainshared.UserID("usr-dave")
	seedQCRecord(t, f.qcRepo, "code-once", domainuser.Authorized, &uid)
	seedQCUser(t, f.userRepo, string(uid))

	var wg sync.WaitGroup
	var minted atomic.Int32
	start := make(chan struct{})
	for range 2 {
		wg.Go(func() {
			<-start
			_, _, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-once", "", false)
			if err == nil && ok {
				minted.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	assert.Equal(t, int32(1), minted.Load())
	assert.Equal(t, 1, f.refreshTkns.Count())
}

// The initiator asked for the secret: a collector without it is refused before
// any token is minted, and the pairing is left intact for the real device.
func TestQCGate_SecretRequired_WrongSecretMintsNothing(t *testing.T) {
	t.Parallel()
	f := newQCGateFixture(t)

	uid := domainshared.UserID("usr-dave")
	require.NoError(t, f.qcRepo.Create(context.Background(), &domainuser.QuickConnectRequest{
		ID: "qcid-secret", Code: "code-secret", Secret: "qc-secret-real", SecretRequired: true,
		Status: domainuser.Authorized, UserID: &uid, ExpiresAt: time.Now().Add(5 * time.Minute),
	}))
	seedQCUser(t, f.userRepo, string(uid))

	_, _, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-secret", "", false)
	require.ErrorIs(t, err, appuser.ErrQuickConnectSecret)
	assert.False(t, ok)
	assert.Equal(t, int64(0), f.userRepo.getByIDCalls.Load(), "no token path without the secret")
	assert.Equal(t, 0, f.refreshTkns.Count())

	access, _, ok, err := f.adapter.checkAndGenerateTokens(context.Background(), "code-secret", "qc-secret-real", false)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.NotEmpty(t, access)
}
