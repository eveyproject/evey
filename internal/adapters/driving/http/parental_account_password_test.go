package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appuser "github.com/eveyproject/evey/internal/app/user"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// accountPasswordFixture wires the parental routes to the real user service so
// the password proof goes through the same lockout as the login form.
func accountPasswordFixture(t *testing.T, passwordHash string) (*http.ServeMux, string) {
	t.Helper()
	gen, _, _ := genTokens(t)
	ctx := context.Background()
	profiles := ufakes.NewFakeProfileRepository()
	_ = profiles.Create(ctx, &domainuser.Profile{ID: "parent", UserID: "u1", Name: "Parent", Parental: &domainuser.ParentalSettings{PINHash: "hashed:1357"}})
	users := ufakes.NewFakeUserRepository()
	_ = users.Create(ctx, &domainuser.User{ID: "u1", Username: "owner", PasswordHash: passwordHash})
	roles := ufakes.NewFakeRoleRepository()
	hasher := ufakes.NewFakePasswordHasher()
	svc := appuser.NewCommandService(users, profiles, ufakes.NewFakeRefreshTokenRepository(), roles,
		ufakes.NewFakeUserRoleRepository(roles), hasher, ufakes.NewFakeTokenGenerator(),
		sharedfakes.NewFakeEventBus(), time.Hour, ufakes.NewFakeQuickConnectRepository())
	mux := http.NewServeMux()
	isolatePINAttempts(registerParentalRoutes(mux, profiles, users, hasher, gen, withPasswordReauth(svc)))
	tok, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	return mux, tok
}

func resetLockoutWith(mux *http.ServeMux, tok, password string) int {
	return do(mux, http.MethodPost, resetLockoutPath, tok, `{"currentPassword":"`+password+`"}`, nil).Code
}

// Wrong passwords count toward the account lockout, and once locked even the
// right password is refused: the proof cannot be ground faster than at login.
func TestParentalAccountPassword_WrongAttemptsLockTheAccount(t *testing.T) {
	mux, tok := accountPasswordFixture(t, "hashed:secret")
	for i := range 10 {
		if code := resetLockoutWith(mux, tok, "wrong"); code != http.StatusForbidden && code != http.StatusTooManyRequests {
			t.Fatalf("attempt %d: %d, want 403 or 429", i, code)
		}
	}
	if code := resetLockoutWith(mux, tok, "secret"); code != http.StatusTooManyRequests {
		t.Fatalf("right password on a locked account: %d, want 429", code)
	}
}

// Surrounding whitespace is trimmed as at login; the right password succeeds.
func TestParentalAccountPassword_RightPasswordTrimmed(t *testing.T) {
	mux, tok := accountPasswordFixture(t, "hashed:secret")
	if code := resetLockoutWith(mux, tok, "  secret "); code != http.StatusNoContent {
		t.Fatalf("padded right password: %d, want 204", code)
	}
}

// An SSO-only account has no password to enter: a distinct answer, not the
// invalid-password one that would send the user guessing.
func TestParentalAccountPassword_SSOOnlyAccountGetsADistinctError(t *testing.T) {
	mux, tok := accountPasswordFixture(t, "")
	w := do(mux, http.MethodPost, resetLockoutPath, tok, `{"currentPassword":"anything"}`, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("SSO-only account: %d, want 409", w.Code)
	}
	if got := w.Body.String(); !strings.Contains(got, "errors.user.no_local_password") {
		t.Fatalf("body %q lacks the no_local_password key", got)
	}
}
