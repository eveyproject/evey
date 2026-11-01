package user

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPINAttempts_FailuresLockAndSuccessResets(t *testing.T) {
	a := NewPINAttempts(nil)
	ctx := context.Background()
	for range pinTiers[0].maxFails {
		assert.False(t, a.Locked(ctx, "prf-a"))
		a.Failed(ctx, "prf-a")
	}
	assert.True(t, a.Locked(ctx, "prf-a"))
	assert.False(t, a.Locked(ctx, "prf-b"), "the count is per profile")

	b := NewPINAttempts(nil)
	b.Failed(ctx, "prf-a")
	b.Succeeded(ctx, "prf-a")
	for range pinTiers[0].maxFails - 1 {
		b.Failed(ctx, "prf-a")
	}
	assert.False(t, b.Locked(ctx, "prf-a"), "a right PIN clears the count")
}

// Each lockout the guesser sits out leads to a longer one.
func TestPINAttempts_LockoutGrows(t *testing.T) {
	saved := pinTiers
	t.Cleanup(func() { pinTiers = saved })
	pinTiers = []pinTier{
		{prefix: "t1:", maxFails: 2, window: 40 * time.Millisecond},
		{prefix: "t2:", maxFails: 4, window: 10 * time.Second},
	}
	a := NewPINAttempts(nil)
	ctx := context.Background()
	a.Failed(ctx, "prf-1")
	a.Failed(ctx, "prf-1")
	require.True(t, a.Locked(ctx, "prf-1"))
	time.Sleep(60 * time.Millisecond)
	require.False(t, a.Locked(ctx, "prf-1"), "the first lockout is short")

	a.Failed(ctx, "prf-1")
	a.Failed(ctx, "prf-1")
	time.Sleep(60 * time.Millisecond)
	assert.True(t, a.Locked(ctx, "prf-1"), "the second one outlasts the first")
}

// HTTP and gRPC check PINs through the same process-wide counter by default.
func TestPINAttempts_SharedByDefault(t *testing.T) {
	first, second := SharedPINAttempts(), SharedPINAttempts()
	assert.Same(t, first, second)
	svc := NewUserService(nil, nil, nil, nil, nil, nil)
	assert.Same(t, SharedPINAttempts(), svc.pinAttempts)
}

func TestSetSharedPINAttempts_IsWhatTheServicesCapture(t *testing.T) {
	saved := SharedPINAttempts()
	t.Cleanup(func() { SetSharedPINAttempts(saved) })
	persisted := NewPINAttempts(newLoginAttemptLimiter(5, time.Minute))
	SetSharedPINAttempts(persisted)
	assert.Same(t, persisted, SharedPINAttempts())
	assert.Same(t, persisted, NewUserService(nil, nil, nil, nil, nil, nil).pinAttempts)
}

// A burst of concurrent guesses cannot all pass the lock check before the
// first failure lands: once the tier fills, the rest are refused unseen.
func TestPINAttempts_ConcurrentGuessesStopAtTheLimit(t *testing.T) {
	a := NewPINAttempts(nil)
	ctx := context.Background()
	var compared atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_, _ = a.Attempt(ctx, []string{"p1"}, "0000", func() (string, error) {
				compared.Add(1)
				time.Sleep(2 * time.Millisecond) // a hash compare takes time
				return "", nil
			})
		})
	}
	wg.Wait()
	if n := compared.Load(); n != 5 {
		t.Fatalf("%d PINs compared, want 5 (the first tier) then refusals", n)
	}
}

// Reset clears a lockout without a PIN; an empty PIN never counts.
func TestPINAttempts_ResetAndEmptyPIN(t *testing.T) {
	a := NewPINAttempts(nil)
	ctx := context.Background()
	none := func() (string, error) { return "", nil }
	for range 10 {
		_, _ = a.Attempt(ctx, []string{"p1"}, "", none)
	}
	if a.Locked(ctx, "p1") {
		t.Fatal("an empty PIN is no guess")
	}
	for range 5 {
		_, _ = a.Attempt(ctx, []string{"p1"}, "0000", none)
	}
	if _, err := a.Attempt(ctx, []string{"p1"}, "0000", none); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("want ErrPINLocked, got %v", err)
	}
	a.Reset(ctx, "p1")
	if a.Locked(ctx, "p1") {
		t.Fatal("Reset must clear the lockout")
	}
}
