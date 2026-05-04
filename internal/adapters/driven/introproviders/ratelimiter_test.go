package introproviders

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When X-RateLimit-Remaining drops below lowMark the rate-limiter should
// block the next call until the X-RateLimit-Reset window has elapsed.
func TestHeaderRateLimiter_WaitsWhenRemaining_BelowLowMark(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var sleptFor time.Duration
	rl := newHeaderRateLimiter()
	rl.nowFunc = func() time.Time { return now }
	rl.sleepFunc = func(_ context.Context, d time.Duration) error {
		sleptFor = d
		return nil
	}

	// Simulate a response that says "1 remaining, reset in 8 seconds".
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "1")
	h.Set("X-RateLimit-Reset", "8")
	rl.Update(h)

	// Wait should ask for an 8-second sleep.
	require.NoError(t, rl.Wait(context.Background()))
	assert.Equal(t, 8*time.Second, sleptFor,
		"should sleep exactly the reset window when remaining < lowMark")
}

// When Remaining is above lowMark, Wait should return immediately without sleeping.
func TestHeaderRateLimiter_NoWait_WhenRemainingAboveLowMark(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	slept := false
	rl := newHeaderRateLimiter()
	rl.nowFunc = func() time.Time { return now }
	rl.sleepFunc = func(_ context.Context, d time.Duration) error {
		slept = true
		return nil
	}

	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "10")
	h.Set("X-RateLimit-Reset", "8")
	rl.Update(h)

	require.NoError(t, rl.Wait(context.Background()))
	assert.False(t, slept, "should not sleep when remaining is above the low-mark threshold")
}

// Wait must respect context cancellation while sleeping.
func TestHeaderRateLimiter_Wait_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := newHeaderRateLimiter()
	rl.nowFunc = func() time.Time { return now }
	// Real sleepFunc so we can test cancellation.
	rl.sleepFunc = func(ctx context.Context, d time.Duration) error {
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "1")
	h.Set("X-RateLimit-Reset", "3600") // 1 hour — would block forever without cancel
	rl.Update(h)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded,
		"Wait must propagate context cancellation")
}
