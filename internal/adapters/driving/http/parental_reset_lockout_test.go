package httpapi

import (
	"context"
	"net/http"
	"testing"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// resetLockoutFixture: an account with a locked-out parent profile and a kids
// profile, plus another account's admin.
func resetLockoutFixture(t *testing.T) (*http.ServeMux, *parentalHandler, func(domainuser.Claims) string) {
	t.Helper()
	gen, _, _ := genTokens(t)
	ctx := context.Background()
	profiles := userfakes.NewFakeProfileRepository()
	_ = profiles.Create(ctx, &domainuser.Profile{ID: "parent", UserID: "u1", Name: "Parent", Parental: &domainuser.ParentalSettings{PINHash: "hashed:1357"}})
	_ = profiles.Create(ctx, &domainuser.Profile{ID: "kid", UserID: "u1", Name: "Kid", IsKids: true})
	_ = profiles.Create(ctx, &domainuser.Profile{ID: "admin-p", UserID: "admin", Name: "Admin"})
	users := userfakes.NewFakeUserRepository()
	_ = users.Create(ctx, &domainuser.User{ID: "u1", Username: "owner", PasswordHash: "hashed:secret"})
	_ = users.Create(ctx, &domainuser.User{ID: "admin", Username: "root", IsAdmin: true, PasswordHash: "hashed:other"})
	mux := http.NewServeMux()
	h := isolatePINAttempts(registerParentalRoutes(mux, profiles, users, userfakes.NewFakePasswordHasher(), gen))
	for range 5 {
		h.pinAttempts.Failed(ctx, "parent")
	}
	tok := func(c domainuser.Claims) string {
		s, err := gen.GenerateAccessToken(c)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	return mux, h, tok
}

const resetLockoutPath = "/api/v1/profiles/parent/parental/reset-lockout"

// A child who locked the parent profile out cannot clear the lockout, and the
// owner needs the account password, not the PIN they cannot type.
func TestParentalResetLockout_OwnerNeedsTheAccountPassword(t *testing.T) {
	mux, h, tok := resetLockoutFixture(t)
	ctx := context.Background()
	kid := tok(domainuser.Claims{UserID: "u1", ProfileID: "kid"})
	if w := do(mux, http.MethodPost, resetLockoutPath, kid, `{"currentPassword":"secret"}`, nil); w.Code != http.StatusForbidden {
		t.Fatalf("from the kids profile: %d, want 403", w.Code)
	}
	owner := tok(domainuser.Claims{UserID: "u1", ProfileID: "parent"})
	if w := do(mux, http.MethodPost, resetLockoutPath, owner, `{"currentPassword":"wrong"}`, nil); w.Code != http.StatusForbidden {
		t.Fatalf("wrong password: %d, want 403", w.Code)
	}
	if !h.pinAttempts.Locked(ctx, "parent") {
		t.Fatal("a refused reset must leave the lockout in place")
	}
	if w := do(mux, http.MethodPost, resetLockoutPath, owner, `{"currentPassword":"secret"}`, nil); w.Code != http.StatusNoContent {
		t.Fatalf("right password: %d, want 204", w.Code)
	}
	if h.pinAttempts.Locked(ctx, "parent") {
		t.Fatal("the lockout must be cleared")
	}
	if code := verifyPINCode(mux, owner, "parent", "1357"); code != http.StatusOK {
		t.Fatalf("PIN after the reset: %d, want 200", code)
	}
}

// An admin clears any profile's lockout without a password; another account's
// non-admin cannot.
func TestParentalResetLockout_AdminClears(t *testing.T) {
	mux, h, tok := resetLockoutFixture(t)
	stranger := tok(domainuser.Claims{UserID: "u2", ProfileID: "x"})
	if w := do(mux, http.MethodPost, resetLockoutPath, stranger, `{"currentPassword":"secret"}`, nil); w.Code != http.StatusForbidden {
		t.Fatalf("another account: %d, want 403", w.Code)
	}
	admin := tok(domainuser.Claims{UserID: "admin", ProfileID: "admin-p", IsAdmin: true})
	if w := do(mux, http.MethodPost, resetLockoutPath, admin, "", nil); w.Code != http.StatusNoContent {
		t.Fatalf("admin: %d, want 204", w.Code)
	}
	if h.pinAttempts.Locked(context.Background(), "parent") {
		t.Fatal("the lockout must be cleared")
	}
}
