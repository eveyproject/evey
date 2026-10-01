package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
)

// AUTH-3: editing one's OWN credentials needs the current password, so a
// stolen access token alone cannot take the account over.

func seedSelf(t *testing.T) *testUserEnv {
	t.Helper()
	env := setupUserHandler()
	seedAdultProfile(t, env, "prf-1", "usr-1")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "oldpassword")
	return env
}

func storedUser(t *testing.T, env *testUserEnv, id string) *user.User {
	t.Helper()
	u, err := env.userRepo.GetByID(context.Background(), shared.UserID(id))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return u
}

func TestUpdateUser_SelfPassword_RequiresCurrent(t *testing.T) {
	newPass, wrong, good := "brandnewpass1", "notmypassword", "oldpassword"
	for _, tc := range []struct {
		name    string
		current *string
		want    codes.Code
	}{
		{"missing", nil, codes.InvalidArgument},
		{"empty", new(string), codes.InvalidArgument},
		{"wrong", &wrong, codes.PermissionDenied},
		{"correct", &good, codes.OK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := seedSelf(t)
			_, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{
				Id: "usr-1", Password: &newPass, CurrentPassword: tc.current,
			})
			hash := storedUser(t, env, "usr-1").PasswordHash
			if tc.want == codes.OK {
				if err != nil {
					t.Fatalf("UpdateUser: %v", err)
				}
				if hash != "hashed:"+newPass {
					t.Fatalf("password not changed: %q", hash)
				}
				return
			}
			assertCode(t, err, tc.want)
			if hash != "hashed:oldpassword" {
				t.Fatalf("password changed despite refusal: %q", hash)
			}
		})
	}
}

func TestUpdateUser_SelfEmail_RequiresCurrent(t *testing.T) {
	mail, wrong, good := "evil@example.com", "notmypassword", "oldpassword"
	for _, tc := range []struct {
		name    string
		current *string
		want    codes.Code
	}{
		{"missing", nil, codes.InvalidArgument},
		{"wrong", &wrong, codes.PermissionDenied},
		{"correct", &good, codes.OK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := seedSelf(t)
			_, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{
				Id: "usr-1", Email: &mail, CurrentPassword: tc.current,
			})
			got := storedUser(t, env, "usr-1").Email
			if tc.want == codes.OK {
				if err != nil {
					t.Fatalf("UpdateUser: %v", err)
				}
				if got != mail {
					t.Fatalf("email not changed: %q", got)
				}
				return
			}
			assertCode(t, err, tc.want)
			if got != "a@b.com" {
				t.Fatalf("email changed despite refusal: %q", got)
			}
		})
	}
}

func TestUpdateUser_SelfUsername_RequiresCurrent(t *testing.T) {
	env := seedSelf(t)
	name := "mallory"
	_, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{Id: "usr-1", Username: &name})
	assertCode(t, err, codes.InvalidArgument)
	if got := storedUser(t, env, "usr-1").Username; got != "alice" {
		t.Fatalf("username changed despite refusal: %q", got)
	}
}

// A form that resends the stored values unchanged is not a credential change.
func TestUpdateUser_SelfUnchangedValues_NoCurrentNeeded(t *testing.T) {
	env := seedSelf(t)
	name, mail, empty := "alice", " a@b.com ", ""
	if _, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{
		Id: "usr-1", Username: &name, Email: &mail, Password: &empty,
	}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
}

func TestUpdateUser_AdminOnOther_NoCurrentNeeded(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "user-1", Username: "admin", Email: "r@b.com", IsAdmin: true}, "adminpassword")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "oldpassword")
	pw, mail := "brandnewpass1", "new@b.com"
	if _, err := env.handler.UpdateUser(adminCtx(), &eveyv1.UpdateUserRequest{Id: "usr-1", Password: &pw, Email: &mail}); err != nil {
		t.Fatalf("admin UpdateUser on another user: %v", err)
	}
	u := storedUser(t, env, "usr-1")
	if u.PasswordHash != "hashed:"+pw || u.Email != mail {
		t.Fatalf("admin change not applied: %q / %q", u.PasswordHash, u.Email)
	}
}

func TestUpdateUser_AdminOnSelf_RequiresCurrent(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "user-1", Username: "admin", Email: "r@b.com", IsAdmin: true}, "adminpassword")
	pw := "brandnewpass1"
	_, err := env.handler.UpdateUser(adminCtx(), &eveyv1.UpdateUserRequest{Id: "user-1", Password: &pw})
	assertCode(t, err, codes.InvalidArgument)
	if h := storedUser(t, env, "user-1").PasswordHash; h != "hashed:adminpassword" {
		t.Fatalf("admin self password changed without current: %q", h)
	}
	good := "adminpassword"
	if _, err := env.handler.UpdateUser(adminCtx(), &eveyv1.UpdateUserRequest{Id: "user-1", Password: &pw, CurrentPassword: &good}); err != nil {
		t.Fatalf("admin self UpdateUser with current: %v", err)
	}
}

// Wrong current passwords feed the login lockout: after the cap even the right
// one is refused, and so is the login form.
func TestUpdateUser_SelfCurrent_SharesLoginLockout(t *testing.T) {
	env := seedSelf(t)
	pw, wrong, good := "brandnewpass1", "notmypassword", "oldpassword"
	for range 5 {
		_, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{Id: "usr-1", Password: &pw, CurrentPassword: &wrong})
		assertCode(t, err, codes.PermissionDenied)
	}
	_, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{Id: "usr-1", Password: &pw, CurrentPassword: &good})
	assertCode(t, err, codes.ResourceExhausted)
	_, err = env.handler.Login(context.Background(), &eveyv1.LoginRequest{Username: "alice", Password: good})
	assertCode(t, err, codes.Unauthenticated)
}

// An SSO-only account has no password to re-enter: refused with a clear
// reason, and an admin can still give it one.
func TestUpdateUser_SelfSSOAccount_NoLocalPassword(t *testing.T) {
	env := setupUserHandler()
	seedAdultProfile(t, env, "prf-1", "usr-1")
	u := &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com", Enabled: true}
	if err := env.userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pw, anything := "brandnewpass1", "whatever"
	_, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{Id: "usr-1", Password: &pw, CurrentPassword: &anything})
	assertCode(t, err, codes.FailedPrecondition)
	if h := storedUser(t, env, "usr-1").PasswordHash; h != "" {
		t.Fatalf("SSO account got a password from its own token: %q", h)
	}

	seedUser(t, env, &user.User{ID: "user-1", Username: "admin", Email: "r@b.com", IsAdmin: true}, "adminpassword")
	if _, err := env.handler.UpdateUser(adminCtx(), &eveyv1.UpdateUserRequest{Id: "usr-1", Password: &pw}); err != nil {
		t.Fatalf("admin sets a password on the SSO account: %v", err)
	}
}
