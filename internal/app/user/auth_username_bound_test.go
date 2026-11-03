package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// countingAttempts counts the failures recorded, whatever the limiter does.
type countingAttempts struct {
	domainuser.LoginAttemptRepository
	records int
}

func (c *countingAttempts) Record(ctx context.Context, username string, maxFails int, window time.Duration) error {
	c.records++
	return c.LoginAttemptRepository.Record(ctx, username, maxFails, window)
}

// A name no account can carry is refused as bad credentials without a row
// written anywhere: the failure counters are keyed on the name as typed, so
// an unbounded one was a free write of any size into them.
func TestLogin_OversizedUsername_RejectedBeforeRecording(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)
	attempts := &countingAttempts{LoginAttemptRepository: newLoginAttemptLimiter(5, 15*time.Minute)}
	d.svc.loginAttempts = attempts

	for _, name := range []string{strings.Repeat("a", domainuser.UsernameMaxRunes+1), "ali\u202ece", "al\x00ice"} {
		_, _, err := d.svc.Login(context.Background(), name, "whatever")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("%q: want ErrInvalidCredentials, got %v", name, err)
		}
	}
	if attempts.records != 0 {
		t.Fatalf("recorded %d failures for names no account can carry, want 0", attempts.records)
	}

	if _, _, err := d.svc.Login(context.Background(), "alice", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a real name with a wrong password: %v", err)
	}
	if attempts.records == 0 {
		t.Fatal("a wrong password on a real name must still be recorded")
	}
}
