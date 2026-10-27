package user

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

func newClaimService(qc *ufakes.FakeQuickConnectRepository) *CommandService {
	roles := ufakes.NewFakeRoleRepository()
	return NewCommandService(ufakes.NewFakeUserRepository(), ufakes.NewFakeProfileRepository(), ufakes.NewFakeRefreshTokenRepository(),
		roles, ufakes.NewFakeUserRoleRepository(roles), ufakes.NewFakePasswordHasher(), ufakes.NewFakeTokenGenerator(),
		sharedfakes.NewFakeEventBus(), time.Hour, qc)
}

// The redeemed state lives in the store, not the process: after a restart (a
// new service on the same store) the code stays spent.
func TestClaimQuickConnect_StaysSpentAcrossARestart(t *testing.T) {
	ctx := context.Background()
	qc := ufakes.NewFakeQuickConnectRepository()
	uid := shared.UserID("u1")
	require.NoError(t, qc.Create(ctx, &user.QuickConnectRequest{ID: "q1", Code: "code-1", Status: user.Authorized, UserID: &uid, ExpiresAt: time.Now().Add(time.Minute)}))

	got, err := newClaimService(qc).ClaimQuickConnect(ctx, "code-1", "")
	require.NoError(t, err)
	require.NotNil(t, got)

	_, err = newClaimService(qc).ClaimQuickConnect(ctx, "code-1", "")
	assert.ErrorIs(t, err, ErrQuickConnectConsumed)
}

// The code is on the screen for anyone to read; the secret only ever went to
// the device that started the pairing. When that device said it would send
// it back, a collector without it gets nothing, and the pairing stays whole
// for the real device.
func TestClaimQuickConnect_SecretRequiredByTheInitiator(t *testing.T) {
	ctx := context.Background()
	qc := ufakes.NewFakeQuickConnectRepository()
	uid := shared.UserID("u1")
	require.NoError(t, qc.Create(ctx, &user.QuickConnectRequest{ID: "q1", Code: "code-1", Secret: "s3cret", SecretRequired: true,
		Status: user.Authorized, UserID: &uid, ExpiresAt: time.Now().Add(time.Minute)}))
	svc := newClaimService(qc)

	_, err := svc.ClaimQuickConnect(ctx, "code-1", "")
	require.ErrorIs(t, err, ErrQuickConnectSecret, "no secret")
	_, err = svc.ClaimQuickConnect(ctx, "code-1", "guess")
	require.ErrorIs(t, err, ErrQuickConnectSecret, "wrong secret")

	got, err := svc.ClaimQuickConnect(ctx, "code-1", "s3cret")
	require.NoError(t, err)
	require.NotNil(t, got, "the refused attempts must not have spent the pairing")
}

// A device that never declared the capability keeps collecting without it.
func TestClaimQuickConnect_LegacyInitiatorNeedsNoSecret(t *testing.T) {
	ctx := context.Background()
	qc := ufakes.NewFakeQuickConnectRepository()
	uid := shared.UserID("u1")
	require.NoError(t, qc.Create(ctx, &user.QuickConnectRequest{ID: "q1", Code: "code-1", Secret: "s3cret",
		Status: user.Authorized, UserID: &uid, ExpiresAt: time.Now().Add(time.Minute)}))

	got, err := newClaimService(qc).ClaimQuickConnect(ctx, "code-1", "")
	require.NoError(t, err)
	require.NotNil(t, got)
}
