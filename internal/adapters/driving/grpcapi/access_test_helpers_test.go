package grpcapi

import (
	appuser "github.com/evey-project/evey/internal/app/user"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
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
