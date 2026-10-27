package user

import (
	"context"
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
