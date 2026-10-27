package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// A non-kids profile with parental control and a PIN is restricted too: from
// it, loosening a PIN-less kids sibling needs the caller's PIN.
func TestParentalUpdateSettings_RestrictedCallerLooseningASiblingNeedsItsPIN(t *testing.T) {
	gen, _, _ := genTokens(t)
	profiles := userfakes.NewFakeProfileRepository()
	twelve, ten := 12, 10
	_ = profiles.Create(context.Background(), &domainuser.Profile{
		ID: "prof-fam", UserID: "user-1", Name: "Family",
		Parental: &domainuser.ParentalSettings{Enabled: true, MaxAge: &twelve, PINHash: "hashed:2468"},
	})
	_ = profiles.Create(context.Background(), &domainuser.Profile{
		ID: "prof-kid", UserID: "user-1", Name: "Kid", IsKids: true,
		Parental: &domainuser.ParentalSettings{Enabled: true, MaxAge: &ten},
	})
	mux := http.NewServeMux()
	isolatePINAttempts(registerParentalRoutes(mux, profiles, userfakes.NewFakeUserRepository(), userfakes.NewFakePasswordHasher(), gen))
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-fam"})
	put := func(body string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-kid/parental", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}
	if code := put(`{"maxAge":18}`); code != http.StatusForbidden {
		t.Fatalf("loosening without the PIN: got %d, want 403", code)
	}
	if code := put(`{"maxAge":18,"currentPin":"0000"}`); code != http.StatusForbidden {
		t.Fatalf("loosening with a wrong PIN: got %d, want 403", code)
	}
	if code := put(`{"maxAge":8}`); code != http.StatusOK {
		t.Fatalf("tightening stays free: got %d", code)
	}
	if code := put(`{"maxAge":18,"currentPin":"2468"}`); code != http.StatusOK {
		t.Fatalf("loosening with the caller's PIN: got %d, want 200", code)
	}
}
