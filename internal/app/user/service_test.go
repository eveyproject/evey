package user_test

import (
	"context"
	"testing"

	appuser "github.com/gravity-zero/evey/internal/app/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
)

func TestSetup(t *testing.T) {
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	rtRepo := userfakes.NewFakeRefreshTokenRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	roleRepo := userfakes.NewFakeRoleRepository()
	userRoleRepo := userfakes.NewFakeUserRoleRepository(roleRepo)
	hasher := userfakes.NewFakePasswordHasher()
	tokenGen := userfakes.NewFakeTokenGenerator()
	bus := sharedfakes.NewFakeEventBus()

	svc := appuser.NewService(userRepo, profileRepo, rtRepo, apiKeyRepo, inviteRepo, qcRepo, roleRepo, userRoleRepo, hasher, tokenGen, bus)

	u, err := svc.Setup(context.Background(), "admin", "password123", "admin@evey.local")
	assert.NoError(t, err)
	assert.Equal(t, "admin", u.Username)
	assert.True(t, u.IsAdmin)

	_, err = svc.Setup(context.Background(), "admin2", "pass", "a2@evey.local")
	assert.ErrorIs(t, err, appuser.ErrSetupDone)
}
