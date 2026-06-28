package grpcapi

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
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

// Regression: a non-admin user granted ZERO libraries must see NO libraries.
// getAllowedLibraryIDs used to collapse a non-nil empty grant list to nil, which
// filterLibrariesByAccess treats as "unrestricted" → the user saw EVERY library
// (authz bypass). The nil result must stay reserved for the admin path.
func TestFilterLibrariesByAccess_ZeroGrantsSeesNone(t *testing.T) {
	t.Parallel()
	userAccess, _ := newAccessServiceWithGrants(t)
	h := &libraryHandler{userAccess: userAccess}
	libs := []*library.Library{{ID: "lib-1"}, {ID: "lib-2"}}

	// Non-admin with no grants → zero libraries.
	nonAdmin := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "u-noaccess", IsAdmin: false})
	got, err := h.filterLibrariesByAccess(nonAdmin, libs)
	require.NoError(t, err)
	assert.Empty(t, got, "non-admin with zero grants must see NO libraries")

	// Admin → unrestricted (sees all).
	admin := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "admin-1", IsAdmin: true})
	gotAdmin, err := h.filterLibrariesByAccess(admin, libs)
	require.NoError(t, err)
	assert.Len(t, gotAdmin, 2, "admin sees all libraries")
}
