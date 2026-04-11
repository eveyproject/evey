package user

import (
	"context"
	"errors"
	"testing"

	"github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	ufakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
)

type userTestDeps struct {
	users       *ufakes.FakeUserRepository
	profiles    *ufakes.FakeProfileRepository
	refreshTkns *ufakes.FakeRefreshTokenRepository
	hasher      *ufakes.FakePasswordHasher
	events      *fakes.FakeEventBus
	svc         *UserService
}

func newUserTestDeps() *userTestDeps {
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	refreshTkns := ufakes.NewFakeRefreshTokenRepository()
	hasher := ufakes.NewFakePasswordHasher()
	events := fakes.NewFakeEventBus()

	svc := NewUserService(users, profiles, refreshTkns, hasher, events)
	return &userTestDeps{users, profiles, refreshTkns, hasher, events, svc}
}

func seedUserSvcUser(t *testing.T, d *userTestDeps, id, username, password string, admin, enabled bool) *user.User {
	t.Helper()
	u := &user.User{
		ID:           id,
		Username:     username,
		PasswordHash: "hashed:" + password,
		IsAdmin:      admin,
		Enabled:      enabled,
	}
	if err := d.users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

// --- CreateUser ---

func TestCreateUser_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	u := &user.User{ID: "u1", Username: "bob"}
	err := d.svc.CreateUser(ctx, true, u, "password123")
	if err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != "hashed:password123" {
		t.Fatalf("expected hashed password, got %s", u.PasswordHash)
	}
	if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps set")
	}

	evts := d.events.PublishedEvents()
	if len(evts) < 1 || evts[0].EventName() != "user.created" {
		t.Fatal("expected user.created event")
	}
	if len(evts) < 2 || evts[1].EventName() != "user.profile.created" {
		t.Fatal("expected user.profile.created event from auto-profile")
	}
}

func TestCreateUser_NotAdmin(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.CreateUser(context.Background(), false, &user.User{}, "password123")
	if !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("expected ErrNotAdmin, got %v", err)
	}
}

func TestCreateUser_InvalidUser(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.CreateUser(context.Background(), true, &user.User{ID: "u1", Username: ""}, "password123")
	if err == nil {
		t.Fatal("expected validation error for empty username")
	}
}

func TestCreateUser_ShortPassword(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.CreateUser(context.Background(), true, &user.User{ID: "u1", Username: "bob"}, "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestCreateUser_HashError(t *testing.T) {
	d := newUserTestDeps()
	d.hasher.Err = errors.New("hash fail")
	err := d.svc.CreateUser(context.Background(), true, &user.User{ID: "u1"}, "password123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateUser_RepoError(t *testing.T) {
	d := newUserTestDeps()
	d.users.Err = errors.New("db fail")
	err := d.svc.CreateUser(context.Background(), true, &user.User{ID: "u1"}, "password123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateUser_EventPublishError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	d.events.PublishErr = errors.New("bus down")
	err := d.svc.CreateUser(context.Background(), true, &user.User{ID: "u1", Username: "bob"}, "password123")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- UpdateUser ---

func TestUpdateUser_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	u := &user.User{ID: "u1", Username: "alice2"}
	err := d.svc.UpdateUser(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if u.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt set")
	}
}

func TestUpdateUser_RepoError(t *testing.T) {
	d := newUserTestDeps()
	d.users.Err = errors.New("fail")
	err := d.svc.UpdateUser(context.Background(), &user.User{ID: "u1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- SetUserBitrateLimit ---

func TestSetUserBitrateLimit_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	err := d.svc.SetUserBitrateLimit(ctx, "u1", 5000)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := d.users.GetByID(ctx, "u1")
	if u.MaxBitrateKbps != 5000 {
		t.Fatalf("expected 5000, got %d", u.MaxBitrateKbps)
	}
}

func TestSetUserBitrateLimit_UserNotFound(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.SetUserBitrateLimit(context.Background(), "ghost", 5000)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ChangePassword ---

func TestChangePassword_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "oldpw", false, true)

	err := d.svc.ChangePassword(ctx, "u1", "newpassword")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := d.users.GetByID(ctx, "u1")
	if u.PasswordHash != "hashed:newpassword" {
		t.Fatalf("expected new hash, got %s", u.PasswordHash)
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.password.changed" {
		t.Fatal("expected password.changed event")
	}
}

func TestChangePassword_TooShort(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.ChangePassword(context.Background(), "u1", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestChangePassword_UserNotFound(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.ChangePassword(context.Background(), "ghost", "longpassword")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChangePassword_HashError(t *testing.T) {
	d := newUserTestDeps()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)
	d.hasher.Err = errors.New("hash fail")
	err := d.svc.ChangePassword(context.Background(), "u1", "longpassword")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChangePassword_UpdateError(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.users.Delete(ctx, "u1")
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)
	d.users.Err = errors.New("update fail")
	err := d.svc.ChangePassword(ctx, "u1", "longpassword")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChangePassword_EventPublishError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)
	d.events.PublishErr = errors.New("bus down")

	err := d.svc.ChangePassword(ctx, "u1", "longpassword")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- DeleteUser ---

func TestDeleteUser_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	err := d.svc.DeleteUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.users.GetByID(ctx, "u1")
	if err == nil {
		t.Fatal("expected user to be deleted")
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.deleted" {
		t.Fatal("expected user.deleted event")
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.DeleteUser(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteUser_LastAdmin(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin", "pw", true, true)

	err := d.svc.DeleteUser(ctx, "u1")
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin, got %v", err)
	}
}

func TestDeleteUser_AdminWithOtherAdmins(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin1", "pw", true, true)
	seedUserSvcUser(t, d, "u2", "admin2", "pw", true, true)

	err := d.svc.DeleteUser(ctx, "u1")
	if err != nil {
		t.Fatalf("should delete admin when other admins exist: %v", err)
	}
}

func TestDeleteUser_CountAdminsError(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin", "pw", true, true)
	d.users.Err = errors.New("db fail")

	err := d.svc.DeleteUser(ctx, "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteUser_DeleteRepoError(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "bob", "pw", false, true)
	d.users.Err = errors.New("delete fail")

	err := d.svc.DeleteUser(ctx, "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteUser_RefreshTokenCleanupError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "bob", "pw", false, true)
	d.refreshTkns.Err = errors.New("cleanup fail")

	err := d.svc.DeleteUser(ctx, "u1")
	if err != nil {
		t.Fatalf("refresh token cleanup error should be non-fatal, got %v", err)
	}
}

func TestDeleteUser_EventPublishError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "bob", "pw", false, true)
	d.events.PublishErr = errors.New("bus down")

	err := d.svc.DeleteUser(ctx, "u1")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- DisableUser ---

func TestDisableUser_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	err := d.svc.DisableUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := d.users.GetByID(ctx, "u1")
	if u.Enabled {
		t.Fatal("expected user disabled")
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.disabled" {
		t.Fatal("expected user.disabled event")
	}
}

func TestDisableUser_NotFound(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.DisableUser(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisableUser_LastAdmin(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin", "pw", true, true)

	err := d.svc.DisableUser(ctx, "u1")
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin, got %v", err)
	}
}

func TestDisableUser_AdminWithOtherAdmins(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin1", "pw", true, true)
	seedUserSvcUser(t, d, "u2", "admin2", "pw", true, true)

	err := d.svc.DisableUser(ctx, "u1")
	if err != nil {
		t.Fatalf("should disable admin when others exist: %v", err)
	}
}

func TestDisableUser_CountAdminsError(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin", "pw", true, true)
	d.users.Err = errors.New("db fail")

	err := d.svc.DisableUser(ctx, "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisableUser_UpdateError(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "bob", "pw", false, true)
	d.users.Err = errors.New("update fail")

	err := d.svc.DisableUser(ctx, "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisableUser_RefreshTokenCleanupError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "bob", "pw", false, true)
	d.refreshTkns.Err = errors.New("cleanup fail")

	err := d.svc.DisableUser(ctx, "u1")
	if err != nil {
		t.Fatalf("refresh token cleanup error should be non-fatal, got %v", err)
	}
}

func TestDisableUser_EventPublishError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "bob", "pw", false, true)
	d.events.PublishErr = errors.New("bus down")

	err := d.svc.DisableUser(ctx, "u1")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- CreateProfile ---

func TestCreateProfile_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	p := &user.Profile{ID: "p1", UserID: "u1", Name: "Main"}
	err := d.svc.CreateProfile(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if p.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt set")
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.profile.created" {
		t.Fatal("expected profile.created event")
	}
}

func TestCreateProfile_ValidationError(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.CreateProfile(context.Background(), &user.Profile{ID: "p1", UserID: "", Name: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCreateProfile_RepoError(t *testing.T) {
	d := newUserTestDeps()
	d.profiles.Err = errors.New("fail")
	err := d.svc.CreateProfile(context.Background(), &user.Profile{ID: "p1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateProfile_EventPublishError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	d.events.PublishErr = errors.New("bus down")
	err := d.svc.CreateProfile(context.Background(), &user.Profile{ID: "p1", UserID: "u1", Name: "M"})
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- UpdateProfile ---

func TestUpdateProfile_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Old"})

	err := d.svc.UpdateProfile(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "New"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateProfile_RepoError(t *testing.T) {
	d := newUserTestDeps()
	d.profiles.Err = errors.New("fail")
	err := d.svc.UpdateProfile(context.Background(), &user.Profile{ID: "p1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- DeleteProfile ---

func TestDeleteProfile_Happy(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Main"})

	err := d.svc.DeleteProfile(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.profile.deleted" {
		t.Fatal("expected profile.deleted event")
	}
}

func TestDeleteProfile_NotFound(t *testing.T) {
	d := newUserTestDeps()
	err := d.svc.DeleteProfile(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteProfile_DeleteRepoError(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "M"})
	d.profiles.Err = errors.New("delete fail")

	err := d.svc.DeleteProfile(ctx, "p1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteProfile_EventPublishError_NonFatal(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "M"})
	d.events.PublishErr = errors.New("bus down")

	err := d.svc.DeleteProfile(ctx, "p1")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- DeleteUser boundary: count <= 1 ---

func TestDeleteUser_AdminBoundary_ExactlyOneAdmin(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin", "pw", true, true)

	err := d.svc.DeleteUser(ctx, "u1")
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("exactly 1 admin should be blocked, got %v", err)
	}
}

func TestDeleteUser_AdminBoundary_TwoAdmins(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin1", "pw", true, true)
	seedUserSvcUser(t, d, "u2", "admin2", "pw", true, true)

	err := d.svc.DeleteUser(ctx, "u1")
	if err != nil {
		t.Fatalf("2 admins should allow delete, got %v", err)
	}
}

// --- DisableUser boundary: count <= 1 ---

func TestDisableUser_AdminBoundary_ExactlyOneAdmin(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin", "pw", true, true)

	err := d.svc.DisableUser(ctx, "u1")
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("exactly 1 admin should be blocked, got %v", err)
	}
}

func TestDisableUser_AdminBoundary_TwoAdmins(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "admin1", "pw", true, true)
	seedUserSvcUser(t, d, "u2", "admin2", "pw", true, true)

	err := d.svc.DisableUser(ctx, "u1")
	if err != nil {
		t.Fatalf("2 admins should allow disable, got %v", err)
	}
}

// --- CreateUser password boundary ---

func TestCreateUser_PasswordBoundary(t *testing.T) {
	t.Run("7 chars rejected", func(t *testing.T) {
		d := newUserTestDeps()
		err := d.svc.CreateUser(context.Background(), true, &user.User{ID: "u1", Username: "bob"}, "1234567")
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("7-char password should be rejected, got %v", err)
		}
	})
	t.Run("8 chars accepted", func(t *testing.T) {
		d := newUserTestDeps()
		err := d.svc.CreateUser(context.Background(), true, &user.User{ID: "u1", Username: "bob"}, "12345678")
		if errors.Is(err, ErrPasswordTooShort) {
			t.Fatal("8-char password should be accepted")
		}
	})
}

func TestChangePassword_RevokesRefreshTokens(t *testing.T) {
	d := newUserTestDeps()
	ctx := context.Background()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{ID: "rt1", UserID: "u1"})
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{ID: "rt2", UserID: "u1"})

	err := d.svc.ChangePassword(ctx, "u1", "newpassword")
	if err != nil {
		t.Fatal(err)
	}
	if d.refreshTkns.Count() != 0 {
		t.Fatalf("expected refresh tokens revoked, got %d", d.refreshTkns.Count())
	}
}

// --- ChangePassword boundary ---

func TestChangePassword_PasswordBoundary(t *testing.T) {
	t.Run("7 chars rejected", func(t *testing.T) {
		d := newUserTestDeps()
		seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)
		err := d.svc.ChangePassword(context.Background(), "u1", "1234567")
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("7-char password should be rejected, got %v", err)
		}
	})
	t.Run("8 chars accepted", func(t *testing.T) {
		d := newUserTestDeps()
		seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)
		err := d.svc.ChangePassword(context.Background(), "u1", "12345678")
		if err != nil {
			t.Fatalf("8-char password should be accepted, got %v", err)
		}
	})
}
