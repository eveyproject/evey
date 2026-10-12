package grpcapi

import (
	"testing"

	appuser "github.com/eveyproject/evey/internal/app/user"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newPermissiveTestUserAccess builds a real *appuser.AccessService backed by
// fakes with an empty FakeLibraryAccessRepository, i.e. no per-user grants
// recorded. GetByUserID on an empty repo returns no restrictions, which every
// production caller in this package reads as "unrestricted" (opt-out default)
// - the same behaviour tests used to get for free from the userAccess == nil
// fail-open guards before those guards were flipped to fail-closed. Handler
// tests that only exercise unrelated code paths should wire this instead of
// leaving userAccess nil, so they keep seeing unfiltered results without
// exercising (or depending on) the fail-closed guard itself.
func newPermissiveTestUserAccess() *appuser.AccessService {
	return appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(userfakes.NewFakeLibraryAccessRepository()),
	)
}

// The library-access rule used to exist as four byte-identical copies across
// the library, music, photo and share handlers. It is now one function, and
// these are the two invariants that must never quietly loosen - both learned
// from real incidents.
func TestLibraryAccessDecision_Invariants(t *testing.T) {
	t.Run("an admin passes even when the access port is unwired", func(t *testing.T) {
		// A wiring gap must never turn an admin's request into a denial: that is
		// the mirror image of the fail-open this guard replaced.
		ok, err := libraryAccessDecision(ctxWithClaims("admin-1", "profile-1", true), nil, "lib-1", "test")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("a normal user is DENIED when the access port is unwired", func(t *testing.T) {
		ok, err := libraryAccessDecision(ctxWithClaims("usr-1", "profile-1", false), nil, "lib-1", "test")
		assert.False(t, ok, "a missing port must deny, never allow every library")
		require.Error(t, err)
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("the status wrapper reports the same two answers", func(t *testing.T) {
		require.NoError(t, libraryAccessOrStatus(ctxWithClaims("admin-1", "p", true), nil, nil, "lib-1", "test"))

		err := libraryAccessOrStatus(ctxWithClaims("usr-1", "p", false), nil, nil, "lib-1", "test")
		require.Error(t, err)
		// The missing-port status must survive the wrapper rather than being
		// flattened into a generic internal error.
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})
}
