package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newTestLimiter(rpm int) *Limiter {
	return &Limiter{
		visitors: make(map[string]*entry),
		rate:     float64(rpm) / 60.0,
		burst:    rpm,
	}
}

func TestRateLimit_AllowsUpToLimit(t *testing.T) {
	l := newTestLimiter(5)
	ip := "10.0.0.1"

	for i := 0; i < 5; i++ {
		assert.True(t, l.Allow(ip), "request %d should be allowed", i+1)
	}
}

func TestRateLimit_BlocksOverLimit(t *testing.T) {
	l := newTestLimiter(3)
	ip := "10.0.0.2"

	for i := 0; i < 3; i++ {
		l.Allow(ip)
	}
	assert.False(t, l.Allow(ip), "request over limit should be blocked")
	assert.False(t, l.Allow(ip), "subsequent request should also be blocked")
}

func TestRateLimit_Refills(t *testing.T) {
	l := newTestLimiter(60) // 1 token/sec
	ip := "10.0.0.3"

	for i := 0; i < 60; i++ {
		l.Allow(ip)
	}
	assert.False(t, l.Allow(ip), "should be exhausted")

	l.mu.Lock()
	e := l.visitors[ip]
	e.lastCheck = e.lastCheck.Add(-2 * time.Second)
	l.mu.Unlock()

	assert.True(t, l.Allow(ip), "should have refilled after elapsed time")
}
