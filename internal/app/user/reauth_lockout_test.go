package user

import (
	"errors"
	"testing"
)

// Re-auth counts on Login's keys: guesses from one address lock that address
// for both, while the owner signing in from elsewhere is untouched.
func TestVerifyCurrentPassword_SharesLoginLockKeys(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)

	for i := 0; i < loginMaxAttempts; i++ {
		if err := d.svc.VerifyCurrentPassword(fromIP("198.51.100.1"), "u1", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	if _, _, err := d.svc.Login(fromIP("198.51.100.1"), "alice", "correct"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("login from the guessing IP: want ErrAccountLocked, got %v", err)
	}
	if err := d.svc.VerifyCurrentPassword(fromIP("198.51.100.1"), "u1", "correct"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("re-auth from the guessing IP: want ErrAccountLocked, got %v", err)
	}
	if _, _, err := d.svc.Login(fromIP("203.0.113.2"), "alice", "correct"); err != nil {
		t.Fatalf("owner from another IP must still sign in: %v", err)
	}
}
