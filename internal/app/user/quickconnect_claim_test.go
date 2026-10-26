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

	got, err := newClaimService(qc).ClaimQuickConnect(ctx, "code-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	_, err = newClaimService(qc).ClaimQuickConnect(ctx, "code-1")
	assert.ErrorIs(t, err, ErrQuickConnectConsumed)
}
