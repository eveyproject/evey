package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// isolatePINAttempts gives a test its own wrong-PIN counter: the process-wide
// one would carry lockouts from one test to the next.
func isolatePINAttempts(h *parentalHandler) *parentalHandler {
	h.pinAttempts = appuser.NewPINAttempts(nil)
	return h
}

func newPINLockoutMux(t *testing.T, profileID string) (*http.ServeMux, string) {
	t.Helper()
	gen, _, _ := genTokens(t)
	profiles := userfakes.NewFakeProfileRepository()
	_ = profiles.Create(context.Background(), &domainuser.Profile{
		ID: shared.ProfileID(profileID), UserID: "user-1", Name: "Parent",
		Parental: &domainuser.ParentalSettings{PINHash: "hashed:1357"},
	})
	mux := http.NewServeMux()
	registerParentalRoutes(mux, profiles, userfakes.NewFakeUserRepository(), userfakes.NewFakePasswordHasher(), gen)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: profileID})
	return mux, token
}

func verifyPINCode(mux *http.ServeMux, token, profileID, pin string) int {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/"+profileID+"/parental/verify-pin", strings.NewReader(`{"pin":"`+pin+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code
}

// Wrong PINs lock the profile: once locked, even the right PIN is refused.
func TestParentalVerifyPIN_WrongPINsLockTheProfile(t *testing.T) {
	const id = "prof-lock-http"
	mux, token := newPINLockoutMux(t, id)
	for range 5 {
		if code := verifyPINCode(mux, token, id, "0000"); code != http.StatusOK {
			t.Fatalf("a wrong PIN answers valid=false, got %d", code)
		}
	}
	if code := verifyPINCode(mux, token, id, "1357"); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once locked, got %d", code)
	}
}

// The gRPC profile RPCs count wrong PINs in the same bucket: failures made
// there lock the HTTP route too.
func TestParentalVerifyPIN_SharesTheLockoutWithGRPC(t *testing.T) {
	const id = "prof-lock-shared"
	mux, token := newPINLockoutMux(t, id)
	svc := appuser.NewUserService(nil, nil, nil, userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(), nil)
	for range 5 {
		_, _ = svc.VerifyProfilePIN(context.Background(), id, "0000", "hashed:1357")
	}
	if code := verifyPINCode(mux, token, id, "1357"); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after wrong PINs over gRPC, got %d", code)
	}
}

// The parent-PIN override of a closed kids profile feeds the same lockout:
// once locked, even the right PIN is refused.
func TestScheduleUnlock_WrongPINsLockTheProfile(t *testing.T) {
	mux, tok, _ := closedKidsFixture(t)
	kid := tok(domainuser.Claims{UserID: "u1", ProfileID: "kid"})
	for range 5 {
		if w := do(mux, http.MethodPost, "/api/v1/profiles/kid/schedule/unlock", kid, `{"pin":"9999"}`, nil); w.Code != http.StatusForbidden {
			t.Fatalf("wrong PIN: %d, want 403", w.Code)
		}
	}
	if w := do(mux, http.MethodPost, "/api/v1/profiles/kid/schedule/unlock", kid, `{"pin":"1234"}`, nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once locked, got %d", w.Code)
	}
}
