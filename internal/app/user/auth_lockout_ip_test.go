package user

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func fromIP(ip string) context.Context { return WithLoginClientIP(context.Background(), ip) }

// Five failures from one address lock that address only: anyone could
// otherwise lock the admin out with five requests every window.
func TestLogin_LockIsPerClientIP(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)

	for i := range loginMaxAttempts {
		if _, _, err := d.svc.Login(fromIP("198.51.100.1"), "alice", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	if _, _, err := d.svc.Login(fromIP("198.51.100.1"), "alice", "correct"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("attacking IP: want ErrAccountLocked, got %v", err)
	}
	if _, _, err := d.svc.Login(fromIP("203.0.113.2"), "Alice", "correct"); err != nil {
		t.Fatalf("other IP with the right password must still get in: %v", err)
	}
}

// One address cannot feed the per-user cap past its own lock: once locked it
// is refused before anything is counted.
func TestLogin_LockedIPDoesNotFeedUserCap(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)
	for range loginUserMaxAttempts + 10 {
		_, _, _ = d.svc.Login(fromIP("198.51.100.1"), "alice", "wrong")
	}
	if _, _, err := d.svc.Login(fromIP("203.0.113.2"), "alice", "correct"); err != nil {
		t.Fatalf("single attacking IP locked the account for everyone: %v", err)
	}
}

// A spread-out guess run still hits a per-user ceiling.
func TestLogin_DistributedFailuresLockTheUser(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)
	for i := range loginUserMaxAttempts {
		_, _, _ = d.svc.Login(fromIP(fmt.Sprintf("198.51.100.%d", i+1)), "alice", "wrong")
	}
	if _, _, err := d.svc.Login(fromIP("203.0.113.2"), "alice", "correct"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked after %d failures over distinct IPs, got %v", loginUserMaxAttempts, err)
	}
}

// No client IP in context: every such caller shares one "unknown" bucket, so
// dropping the IP is no way around the lock.
func TestLogin_MissingClientIPSharesOneBucket(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)
	for range loginMaxAttempts {
		_, _, _ = d.svc.Login(context.Background(), "alice", "wrong")
	}
	for name, ctx := range map[string]context.Context{"no value": context.Background(), "empty": fromIP(""), "garbage": fromIP("not-an-ip")} {
		if _, _, err := d.svc.Login(ctx, "alice", "correct"); !errors.Is(err, ErrAccountLocked) {
			t.Fatalf("%s: want ErrAccountLocked, got %v", name, err)
		}
	}
	if _, _, err := d.svc.Login(fromIP("203.0.113.2"), "alice", "correct"); err != nil {
		t.Fatalf("a real IP is a different bucket: %v", err)
	}
}

func TestLoginAttemptKeys_Unambiguous(t *testing.T) {
	pair, usr := loginAttemptKeys(fromIP("198.51.100.1"), " Alice ")
	if pair != "ip:198.51.100.1|user:alice" || usr != "user:alice" {
		t.Fatalf("keys = %q, %q", pair, usr)
	}
	if pair, _ := loginAttemptKeys(context.Background(), "alice"); pair != "ip:unknown|user:alice" {
		t.Fatalf("no-IP pair key = %q", pair)
	}
}

// The extended demo's guest account is never locked, by address or by
// account: a mistyped password on a TV remote must not close the only door.
// Everyone else keeps the lock.
func TestLogin_LockExemptAccountIsNeverLocked(t *testing.T) {
	d := newAuthTestDeps()
	CommandWithLoginLockExempt("Guest1")(d.svc)
	seedAuthUser(t, d, "u1", "guest1", "correct", true, true)
	seedAuthUser(t, d, "u2", "alice", "correct", false, true)

	for range loginUserMaxAttempts + 10 {
		_, _, _ = d.svc.Login(fromIP("198.51.100.1"), "guest1", "wrong")
		_, _, _ = d.svc.Login(fromIP("198.51.100.1"), "alice", "wrong")
	}
	if _, _, err := d.svc.Login(fromIP("198.51.100.1"), "guest1", "correct"); err != nil {
		t.Fatalf("exempt account must still sign in after a failure run: %v", err)
	}
	if _, _, err := d.svc.Login(fromIP("198.51.100.1"), "alice", "correct"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("a non-exempt account must stay lockable, got %v", err)
	}
}
