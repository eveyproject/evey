package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/user"
)

// The initiator fixes whether the secret is demanded; the collector cannot
// opt out. Without it the check answers like an unknown code, so a third
// party who scanned the QR learns nothing and spends nothing.
func TestCheckQuickConnect_SecretAwareInitiatorRequiresTheSecret(t *testing.T) {
	env := setupUserHandler()
	seedAdultProfile(t, env, "prf-1", "usr-1")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "tester"}, "pass")
	initResp, err := env.handler.InitiateQuickConnect(context.Background(), &eveyv1.InitiateQuickConnectRequest{SecretAware: true})
	require.NoError(t, err)
	require.NotEmpty(t, initResp.Secret)
	_, err = env.handler.AuthorizeQuickConnect(userCtx("usr-1", "prf-1"), &eveyv1.AuthorizeQuickConnectRequest{Code: initResp.Code})
	require.NoError(t, err)

	_, err = env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: initResp.Code})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: initResp.Code, Secret: "not-it"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	resp, err := env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: initResp.Code, Secret: initResp.Secret})
	require.NoError(t, err)
	assert.True(t, resp.Authorized)
	assert.NotEmpty(t, resp.GetAuth().GetAccessToken(), "the real device still collects after the refused attempts")
}

// An installed client that predates the field keeps pairing as before.
func TestCheckQuickConnect_LegacyInitiatorCollectsWithoutSecret(t *testing.T) {
	env := setupUserHandler()
	seedAdultProfile(t, env, "prf-1", "usr-1")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "tester"}, "pass")
	initResp, err := env.handler.InitiateQuickConnect(context.Background(), &eveyv1.InitiateQuickConnectRequest{})
	require.NoError(t, err)
	_, err = env.handler.AuthorizeQuickConnect(userCtx("usr-1", "prf-1"), &eveyv1.AuthorizeQuickConnectRequest{Code: initResp.Code})
	require.NoError(t, err)

	resp, err := env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: initResp.Code})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetAuth().GetAccessToken())
}
