package user

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// recordingRevoker records Revoke calls for assertions.
type recordingRevoker struct {
	revoked []string
}

func (r *recordingRevoker) Revoke(_ context.Context, userID string) error {
	r.revoked = append(r.revoked, userID)
	return nil
}

func newRevokeTestDeps() (*userTestDeps, *recordingRevoker) {
	d := newUserTestDeps()
	rev := &recordingRevoker{}
	WithTokenRevoker(rev)(d.svc)
	return d, rev
}

func TestDisableUser_RevokesAccessTokens(t *testing.T) {
	d, rev := newRevokeTestDeps()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	if err := d.svc.DisableUser(context.Background(), "u1"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if len(rev.revoked) != 1 || rev.revoked[0] != "u1" {
		t.Errorf("want one revoke for u1, got %v", rev.revoked)
	}
}

func TestChangePassword_RevokesAccessTokens(t *testing.T) {
	d, rev := newRevokeTestDeps()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	if err := d.svc.ChangePassword(context.Background(), "u1", "newpassword123"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if len(rev.revoked) != 1 || rev.revoked[0] != "u1" {
		t.Errorf("want one revoke for u1, got %v", rev.revoked)
	}
}

func TestUpdateUser_RoleChangeRevokes_PlainEditDoesNot(t *testing.T) {
	d, rev := newRevokeTestDeps()
	u := seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)

	// Plain edit (no role change) must not nuke the user's sessions. Use a
	// copy: real handlers build a fresh User from the request, and the fake
	// repo stores pointers.
	plain := *u
	plain.Email = "alice@example.com"
	if err := d.svc.UpdateUser(context.Background(), &plain); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(rev.revoked) != 0 {
		t.Fatalf("plain edit should not revoke, got %v", rev.revoked)
	}

	// Promotion/demotion changes the is_admin claim baked into outstanding
	// JWTs → must revoke.
	promoted := plain
	promoted.IsAdmin = true
	if err := d.svc.UpdateUser(context.Background(), &promoted); err != nil {
		t.Fatalf("update role: %v", err)
	}
	if len(rev.revoked) != 1 || rev.revoked[0] != "u1" {
		t.Errorf("want one revoke for u1 after role change, got %v", rev.revoked)
	}
}

func TestRevokeUserTokens_DeletesRefreshAndBumps(t *testing.T) {
	d, rev := newRevokeTestDeps()
	seedUserSvcUser(t, d, "u1", "alice", "pw", false, true)
	if err := d.refreshTkns.Create(context.Background(), &user.RefreshToken{
		ID: "rt1", UserID: shared.UserID("u1"), TokenHash: "h1",
		ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := d.svc.RevokeUserTokens(context.Background(), "u1"); err != nil {
		t.Fatalf("revoke user tokens: %v", err)
	}
	if len(rev.revoked) != 1 || rev.revoked[0] != "u1" {
		t.Errorf("want one epoch revoke for u1, got %v", rev.revoked)
	}
	if n := d.refreshTkns.Count(); n != 0 {
		t.Errorf("want refresh tokens deleted, got %d", n)
	}
}
