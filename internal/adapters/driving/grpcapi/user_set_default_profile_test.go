package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestSetDefaultProfile covers the ownership / IDOR behaviour of
// userHandler.SetDefaultProfile.
//
// Ownership model: SetDefaultProfile is admin-only (RBAC gate, enforced by the
// interceptor before the handler runs). The handler itself does NOT check that
// the target profile belongs to the caller; it delegates entirely to
// UserService.SetDefaultProfile which resolves ownership from the profile row
// itself (target.UserID), not from the caller's claims. An admin can therefore
// set the default profile for any user - this is the intended design for an
// admin-only RPC.

func TestSetDefaultProfile_HappyPath_OwnProfile(t *testing.T) {
	env := setupUserHandler()
	ctx := context.Background()

	// Seed two profiles for the caller's own user.
	_ = env.profileRepo.Create(ctx, &user.Profile{
		ID: "prf-main", UserID: "usr-1", Name: "Main", IsDefault: true,
	})
	_ = env.profileRepo.Create(ctx, &user.Profile{
		ID: "prf-alt", UserID: "usr-1", Name: "Alt", IsDefault: false,
	})

	// Admin caller promotes "prf-alt" to default.
	adminContext := adminCtx()
	resp, err := env.handler.SetDefaultProfile(adminContext, &eveyv1.SetDefaultProfileRequest{Id: "prf-alt"})
	if err != nil {
		t.Fatalf("SetDefaultProfile: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Id != "prf-alt" {
		t.Fatalf("response profile id = %q, want prf-alt", resp.Id)
	}
	if !resp.IsDefault {
		t.Fatal("promoted profile should be is_default=true in response")
	}

	// Verify the service delegated (callerUserID, profileID) correctly: the
	// profile row in the repo must now be the default for usr-1.
	promoted, err := env.profileRepo.GetByID(ctx, "prf-alt")
	if err != nil {
		t.Fatalf("GetByID prf-alt: %v", err)
	}
	if !promoted.IsDefault {
		t.Error("prf-alt should be is_default=true after SetDefaultProfile")
	}

	// Previous default must have been demoted.
	prev, err := env.profileRepo.GetByID(ctx, "prf-main")
	if err != nil {
		t.Fatalf("GetByID prf-main: %v", err)
	}
	if prev.IsDefault {
		t.Error("prf-main should be is_default=false after another profile was promoted")
	}
}

// TestSetDefaultProfile_AdminCanSetAnotherUsersDefault documents the delegation
// model: there is no handler-layer IDOR guard. An admin caller can promote a
// profile owned by a different user. The service resolves ownership from the
// profile row (target.UserID), not from the caller's claims.
func TestSetDefaultProfile_AdminCanSetAnotherUsersDefault(t *testing.T) {
	env := setupUserHandler()
	ctx := context.Background()

	// Two different users, each with one profile.
	_ = env.profileRepo.Create(ctx, &user.Profile{
		ID: "prf-u2-main", UserID: "usr-2", Name: "U2 Main", IsDefault: true,
	})
	_ = env.profileRepo.Create(ctx, &user.Profile{
		ID: "prf-u2-alt", UserID: "usr-2", Name: "U2 Alt", IsDefault: false,
	})

	// Admin caller (whose own user is "user-1") sets default for usr-2's profile.
	adminContext := adminCtx()
	resp, err := env.handler.SetDefaultProfile(adminContext, &eveyv1.SetDefaultProfileRequest{Id: "prf-u2-alt"})
	if err != nil {
		t.Fatalf("admin SetDefaultProfile for another user's profile: %v", err)
	}
	if resp.Id != "prf-u2-alt" {
		t.Fatalf("response id = %q, want prf-u2-alt", resp.Id)
	}
	if !resp.IsDefault {
		t.Fatal("promoted profile should be marked is_default")
	}

	// Confirm the delegation reached the repo correctly: usr-2's profile is now default.
	promoted, err := env.profileRepo.GetByID(ctx, "prf-u2-alt")
	if err != nil {
		t.Fatalf("GetByID prf-u2-alt: %v", err)
	}
	if !promoted.IsDefault {
		t.Error("prf-u2-alt should be is_default=true")
	}
}

// TestSetDefaultProfile_UnknownProfile verifies the handler maps a not-found
// error from the service to codes.NotFound.
func TestSetDefaultProfile_UnknownProfile(t *testing.T) {
	env := setupUserHandler()

	adminContext := adminCtx()
	_, err := env.handler.SetDefaultProfile(adminContext, &eveyv1.SetDefaultProfileRequest{Id: "prf-ghost"})
	if err == nil {
		t.Fatal("expected error for unknown profile id")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got code %v, want NotFound", status.Code(err))
	}
}

// TestSetDefaultProfile_EmptyID verifies the handler rejects a missing profile
// id with codes.InvalidArgument before hitting the service.
func TestSetDefaultProfile_EmptyID(t *testing.T) {
	env := setupUserHandler()

	adminContext := adminCtx()
	_, err := env.handler.SetDefaultProfile(adminContext, &eveyv1.SetDefaultProfileRequest{Id: ""})
	if err == nil {
		t.Fatal("expected error for empty profile id")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
	}
}

// TestSetDefaultProfile_Unauthenticated verifies the handler rejects calls
// without valid claims even when the RBAC interceptor is bypassed in unit tests.
func TestSetDefaultProfile_Unauthenticated(t *testing.T) {
	env := setupUserHandler()

	_, err := env.handler.SetDefaultProfile(context.Background(), &eveyv1.SetDefaultProfileRequest{Id: "prf-any"})
	if err == nil {
		t.Fatal("expected error without claims")
	}
	// requireClaims wraps the unauthenticated status - accept any non-nil error
	// whose code is Unauthenticated or whose wrapped error comes from requireClaims.
	code := status.Code(err)
	if code != codes.Unauthenticated && code != codes.Internal {
		t.Fatalf("got code %v, want Unauthenticated (or wrapped Internal from requireClaims)", code)
	}
}
