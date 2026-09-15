package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
)

// ImpersonateUser is god-mode: the token it mints must carry the target's
// identity AND the acting admin, and every refusal must be a precise code.
func TestImpersonateUser(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "adm-1", Username: "admin", IsAdmin: true}, "pw")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice"}, "pw")
	seedUser(t, env, &user.User{ID: "usr-off", Username: "off"}, "pw")
	if err := env.userSvc.DisableUser(context.Background(), "usr-off"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*user.Profile{
		{ID: "prf-1", UserID: "usr-1", Name: "alice", IsDefault: true},
		{ID: "prf-2", UserID: "usr-1", Name: "kid"},
		{ID: "prf-other", UserID: "adm-1", Name: "admin", IsDefault: true},
	} {
		if err := env.profileRepo.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	admin := ctxWithClaims("adm-1", "prf-other", true)

	// Nominal: default profile when none is asked for.
	resp, err := env.handler.ImpersonateUser(admin, &eveyv1.ImpersonateUserRequest{UserId: "usr-1"})
	if err != nil {
		t.Fatalf("impersonate: %v", err)
	}
	claims, err := env.tokenGen.ValidateAccessToken(resp.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "usr-1" || claims.ProfileID != "prf-1" || claims.IsAdmin {
		t.Errorf("claims = %+v, want alice on her default profile, not admin", claims)
	}
	if claims.ImpersonatedBy != "adm-1" {
		t.Errorf("ImpersonatedBy = %q, want the acting admin", claims.ImpersonatedBy)
	}

	// A specific profile of the target.
	resp, err = env.handler.ImpersonateUser(admin, &eveyv1.ImpersonateUserRequest{UserId: "usr-1", ProfileId: "prf-2"})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := env.tokenGen.ValidateAccessToken(resp.AccessToken); c.ProfileID != "prf-2" {
		t.Errorf("ProfileID = %q, want prf-2", c.ProfileID)
	}

	// Refusals.
	_, err = env.handler.ImpersonateUser(admin, &eveyv1.ImpersonateUserRequest{})
	assertCode(t, err, codes.InvalidArgument)
	_, err = env.handler.ImpersonateUser(admin, &eveyv1.ImpersonateUserRequest{UserId: "ghost"})
	assertCode(t, err, codes.NotFound)
	_, err = env.handler.ImpersonateUser(admin, &eveyv1.ImpersonateUserRequest{UserId: "usr-off"})
	assertCode(t, err, codes.FailedPrecondition)
	// Someone else's profile must not be attachable to the target's identity.
	_, err = env.handler.ImpersonateUser(admin, &eveyv1.ImpersonateUserRequest{UserId: "usr-1", ProfileId: "prf-other"})
	assertCode(t, err, codes.NotFound)
	// No claims at all.
	_, err = env.handler.ImpersonateUser(context.Background(), &eveyv1.ImpersonateUserRequest{UserId: "usr-1"})
	if err == nil {
		t.Error("an unauthenticated context must be refused")
	}
}

// ValidateInvite is the pre-registration probe: it never errors on a bad code
// (no oracle beyond valid/invalid), and it reports the admin flag of a usable
// invite so the sign-up form can say what the account will be.
func TestValidateInvite(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "adm-1", Username: "admin", IsAdmin: true}, "pw")
	admin := ctxWithClaims("adm-1", "prf-adm", true)

	plain, err := env.handler.CreateInviteLink(admin, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	elevated, err := env.handler.CreateInviteLink(admin, &eveyv1.CreateInviteLinkRequest{MaxUses: 1, IsAdmin: true})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := env.handler.ValidateInvite(context.Background(), &eveyv1.ValidateInviteRequest{Code: plain.Code})
	if err != nil || !resp.Valid || resp.IsAdmin {
		t.Errorf("plain invite: resp=%+v err=%v, want valid and not admin", resp, err)
	}
	resp, err = env.handler.ValidateInvite(context.Background(), &eveyv1.ValidateInviteRequest{Code: elevated.Code})
	if err != nil || !resp.Valid || !resp.IsAdmin {
		t.Errorf("admin invite: resp=%+v err=%v, want valid and admin", resp, err)
	}
	resp, err = env.handler.ValidateInvite(context.Background(), &eveyv1.ValidateInviteRequest{Code: "no-such-code"})
	if err != nil || resp.Valid || resp.IsAdmin {
		t.Errorf("unknown code: resp=%+v err=%v, want invalid without error", resp, err)
	}

	// A revoked invite stops validating.
	if _, revokeErr := env.handler.RevokeInviteLink(admin, &eveyv1.RevokeInviteLinkRequest{Id: plain.Id}); revokeErr != nil {
		t.Fatal(revokeErr)
	}
	resp, err = env.handler.ValidateInvite(context.Background(), &eveyv1.ValidateInviteRequest{Code: plain.Code})
	if err != nil || resp.Valid {
		t.Errorf("revoked invite: resp=%+v err=%v, want invalid", resp, err)
	}
}
