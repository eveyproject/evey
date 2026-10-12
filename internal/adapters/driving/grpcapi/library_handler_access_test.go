package grpcapi

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAccessServiceWithGrants(t *testing.T) (*appuser.AccessService, *userfakes.FakeLibraryAccessRepository) {
	t.Helper()
	repo := userfakes.NewFakeLibraryAccessRepository()
	svc := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(repo),
	)
	return svc, repo
}

// A non-admin's library grants are an opt-out WHITELIST: zero grants means
// "unrestricted" (sees every library, the admin-panel default when no box is
// checked), and grants only restrict once present. This mirrors HasAccess
// (zero restrictions = full access) so the library list, home feeds and the
// per-item check all agree.
func TestFilterLibrariesByAccess_ZeroGrantsSeesAll(t *testing.T) {
	t.Parallel()
	userAccess, repo := newAccessServiceWithGrants(t)
	h := &libraryHandler{userAccess: userAccess}
	libs := []*library.Library{{ID: "lib-1"}, {ID: "lib-2"}}

	// Non-admin with NO grants → unrestricted (sees all libraries).
	nonAdmin := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "u-noaccess", IsAdmin: false})
	got, err := h.filterLibrariesByAccess(nonAdmin, libs)
	require.NoError(t, err)
	assert.Len(t, got, 2, "non-admin with zero grants sees ALL libraries (opt-out default)")

	// Non-admin WITH an explicit grant → restricted to that library only.
	require.NoError(t, repo.Grant(context.Background(), shared.UserID("u-restricted"), shared.LibraryID("lib-1")))
	restricted := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "u-restricted", IsAdmin: false})
	gotRestricted, err := h.filterLibrariesByAccess(restricted, libs)
	require.NoError(t, err)
	require.Len(t, gotRestricted, 1, "granted non-admin sees only whitelisted libraries")
	assert.Equal(t, shared.LibraryID("lib-1"), gotRestricted[0].ID)

	// Admin → unrestricted (sees all).
	admin := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "admin-1", IsAdmin: true})
	gotAdmin, err := h.filterLibrariesByAccess(admin, libs)
	require.NoError(t, err)
	assert.Len(t, gotAdmin, 2, "admin sees all libraries")
}
