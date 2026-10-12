package user

import (
	"context"
	"testing"

	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

func TestLibraryAccess_UserWithAccess_Authorized(t *testing.T) {
	repo := ufakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	_ = repo.Grant(ctx, "user-1", "lib-movies")

	ok, err := repo.HasAccess(ctx, "user-1", "lib-movies")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("user with granted access should be authorized")
	}
}

func TestLibraryAccess_UserWithoutAccess_Denied(t *testing.T) {
	repo := ufakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	// user has restriction (granted to lib-movies only)
	_ = repo.Grant(ctx, "user-1", "lib-movies")

	ok, err := repo.HasAccess(ctx, "user-1", "lib-tvshows")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("user without access to lib-tvshows should be denied")
	}
}

func TestLibraryAccess_NoRestrictions_FullAccess(t *testing.T) {
	repo := ufakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	// no grants = no restrictions = full access (simulates admin or unrestricted user)
	ok, err := repo.HasAccess(ctx, "user-1", "any-lib")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("user with no restrictions should have full access")
	}
}
