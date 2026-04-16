package ratelimit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// SetLimit updates the refill rate and burst; new IPs use the new values.
func TestLimiter_SetLimit_UpdatesRateAndBurst(t *testing.T) {
	l := New(60) // 1 rps
	defer l.Stop()

	// Consume the burst at 60 rpm.
	for i := 0; i < 60; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("Allow #%d: want true", i)
		}
	}
	// Over the initial burst → blocked.
	assert.False(t, l.Allow("1.2.3.4"))

	// Bump to 120 rpm; a fresh IP gets 120 tokens.
	l.SetLimit(120)
	for i := 0; i < 120; i++ {
		if !l.Allow("5.6.7.8") {
			t.Fatalf("Allow fresh IP #%d after SetLimit: want true", i)
		}
	}
	assert.False(t, l.Allow("5.6.7.8"), "should be blocked after exhausting new burst")
}
