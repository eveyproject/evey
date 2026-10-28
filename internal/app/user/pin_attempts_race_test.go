package user

import (
	"context"
	"sync"
	"testing"
	"time"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// slowHasher is the fake hasher with a bcrypt-like delay on Verify, so
// concurrent guesses overlap the way they do against the real hash.
type slowHasher struct {
	*ufakes.FakePasswordHasher
	mu    sync.Mutex
	calls int
}

func (h *slowHasher) Verify(password, hash string) (bool, error) {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()
	time.Sleep(2 * time.Millisecond)
	return h.FakePasswordHasher.Verify(password, hash)
}

// The gRPC current-PIN check has no other throttle than the lockout: a burst
// of concurrent wrong PINs must stop at the first tier, not all reach the
// hash compare before the first failure is recorded.
func TestVerifyProfilePIN_ConcurrentBurstStopsAtTheLockout(t *testing.T) {
	h := &slowHasher{FakePasswordHasher: ufakes.NewFakePasswordHasher()}
	svc := NewUserService(nil, nil, nil, h, sharedfakes.NewFakeEventBus(), nil, WithPINAttempts(NewPINAttempts(nil)))
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.VerifyProfilePIN(context.Background(), "p-burst", "0000", "hashed:1357")
		}()
	}
	wg.Wait()
	if h.calls != 5 {
		t.Fatalf("%d PINs compared during the burst, want 5", h.calls)
	}
}
