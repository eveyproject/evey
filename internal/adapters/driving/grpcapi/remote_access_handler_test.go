package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newRemoteAccessHandler() *remoteAccessHandler {
	return &remoteAccessHandler{
		cfg: &config.Config{
			RemoteAccess: config.RemoteAccessConfig{Enabled: false},
			Server:       config.ServerConfig{BaseURL: "http://localhost"},
		},
	}
}

func TestRemoteAccess_UpdateRemoteAccess_NonAdmin_PermissionDenied(t *testing.T) {
	h := newRemoteAccessHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	enabled := true
	_, err := h.UpdateRemoteAccess(ctx, &eveyv1.UpdateRemoteAccessRequest{Enabled: &enabled})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRemoteAccess_UpdateRemoteAccess_Unauthenticated(t *testing.T) {
	h := newRemoteAccessHandler()
	enabled := true
	_, err := h.UpdateRemoteAccess(context.Background(), &eveyv1.UpdateRemoteAccessRequest{Enabled: &enabled})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestRemoteAccess_UpdateRemoteAccess_Admin_Success(t *testing.T) {
	h := newRemoteAccessHandler()
	ctx := ctxWithClaims("admin-1", "profile-admin", true)
	enabled := true
	resp, err := h.UpdateRemoteAccess(ctx, &eveyv1.UpdateRemoteAccessRequest{Enabled: &enabled})
	require.NoError(t, err)
	assert.True(t, resp.Enabled)
	assert.True(t, h.cfg.RemoteAccess.Enabled)
}
